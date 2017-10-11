package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/metaleap/go-util-slice"
	"github.com/metaleap/go-util-str"
)

const (
	nsPrefixDefaultFfiPkg  = "Ps2GoFFI."
	saniUpperToLowerPrefix = "µˇ"
)

var (
	sanitizer = strings.NewReplacer("'", "ˇ", "$", "Ø")
)

type GIrANamedTypeRefs []*GIrANamedTypeRef

func (me GIrANamedTypeRefs) ByPsName(psname string) *GIrANamedTypeRef {
	for _, gntr := range me {
		if gntr.NamePs == psname {
			return gntr
		}
	}
	return nil
}

func (me GIrANamedTypeRefs) Eq(cmp GIrANamedTypeRefs) bool {
	if l := len(me); l != len(cmp) {
		return false
	} else {
		for i := 0; i < l; i++ {
			if !me[i].Eq(cmp[i]) {
				return false
			}
		}
	}
	return true
}

type GIrANamedTypeRef struct {
	NamePs string `json:",omitempty"`
	NameGo string `json:",omitempty"`

	RefAlias     string                `json:",omitempty"`
	RefUnknown   int                   `json:",omitempty"`
	RefInterface *GIrATypeRefInterface `json:",omitempty"`
	RefFunc      *GIrATypeRefFunc      `json:",omitempty"`
	RefStruct    *GIrATypeRefStruct    `json:",omitempty"`
	RefArray     *GIrATypeRefArray     `json:",omitempty"`
	RefPtr       *GIrATypeRefPtr       `json:",omitempty"`

	EnumConstNames []string          `json:",omitempty"`
	Methods        GIrANamedTypeRefs `json:",omitempty"`
	Export         bool              `json:",omitempty"`
	WasTypeFunc    bool              `json:",omitempty"`

	method  GIrATypeMethod
	ctor    *GIrMTypeDataCtor
	comment *GIrAComments
	instOf  string
}

type GIrATypeMethod struct {
	body      *GIrABlock
	isNewCtor bool
	hasNoThis bool
}

func (me *GIrANamedTypeRef) Eq(cmp *GIrANamedTypeRef) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.RefAlias == cmp.RefAlias && me.RefUnknown == cmp.RefUnknown && me.RefInterface.Eq(cmp.RefInterface) && me.RefFunc.Eq(cmp.RefFunc) && me.RefStruct.Eq(cmp.RefStruct) && me.RefArray.Eq(cmp.RefArray) && me.RefPtr.Eq(cmp.RefPtr))
}

func (me *GIrANamedTypeRef) setBothNamesFromPsName(psname string) {
	me.NamePs = psname
	me.NameGo = sanitizeSymbolForGo(psname, me.Export || me.WasTypeFunc)
}

func (me *GIrANamedTypeRef) setRefFrom(tref interface{}) {
	switch tr := tref.(type) {
	case *GIrANamedTypeRef:
		me.RefAlias = tr.RefAlias
		me.RefArray = tr.RefArray
		me.RefFunc = tr.RefFunc
		me.RefInterface = tr.RefInterface
		me.RefPtr = tr.RefPtr
		me.RefStruct = tr.RefStruct
		me.RefUnknown = tr.RefUnknown
	case *GIrATypeRefInterface:
		me.RefInterface = tr
	case *GIrATypeRefFunc:
		me.RefFunc = tr
	case *GIrATypeRefStruct:
		me.RefStruct = tr
	case *GIrATypeRefArray:
		me.RefArray = tr
	case *GIrATypeRefPtr:
		me.RefPtr = tr
	case int:
		me.RefUnknown = tr
	case string:
		me.RefAlias = tr
	case nil:
		me.RefAlias = "interface{/*setRefFrom*/}"
	default:
		println(tref.(float32)) // in case of future oversight, trigger immediate panic-msg with actual-type included
	}
}

type GIrATypeRefArray struct {
	Of *GIrANamedTypeRef
}

func (me *GIrATypeRefArray) Eq(cmp *GIrATypeRefArray) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Of.Eq(cmp.Of))
}

type GIrATypeRefPtr struct {
	Of *GIrANamedTypeRef
}

func (me *GIrATypeRefPtr) Eq(cmp *GIrATypeRefPtr) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Of.Eq(cmp.Of))
}

type GIrATypeRefInterface struct {
	Embeds  []string          `json:",omitempty"`
	Methods GIrANamedTypeRefs `json:",omitempty"`

	xtc              *GIrMTypeClass
	xtd              *GIrMTypeDataDecl
	inheritedMethods GIrANamedTypeRefs
}

func (me *GIrATypeRefInterface) Eq(cmp *GIrATypeRefInterface) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && uslice.StrEq(me.Embeds, cmp.Embeds) && me.Methods.Eq(cmp.Methods))
}

func (me *GIrATypeRefInterface) allMethods() (allmethods GIrANamedTypeRefs) {
	allmethods = me.Methods
	if (!areOverlappingInterfacesSupportedByGo) && len(me.Embeds) > 0 {
		if len(me.inheritedMethods) == 0 {
			m := map[string]*GIrANamedTypeRef{}
			for _, embed := range me.Embeds {
				if gtd := findGoTypeByPsQName(embed); gtd == nil || gtd.RefInterface == nil {
					panic(fmt.Errorf("%s: references unknown interface/type-class %s, please report!", me.xtc.Name, embed))
				} else {
					for _, method := range gtd.RefInterface.allMethods() {
						if dupl, _ := m[method.NameGo]; dupl == nil {
							m[method.NameGo], me.inheritedMethods = method, append(me.inheritedMethods, method)
						} else if !dupl.Eq(method) {
							panic("Interface (generated from type-class " + me.xtc.Name + ") would inherit multiple (but different-signature) methods named " + method.NameGo)
						}
					}
				}
			}
		}
		allmethods = append(me.inheritedMethods, allmethods...)
	}
	return
}

type GIrATypeRefFunc struct {
	Args GIrANamedTypeRefs `json:",omitempty"`
	Rets GIrANamedTypeRefs `json:",omitempty"`
}

func (me *GIrATypeRefFunc) Eq(cmp *GIrATypeRefFunc) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && me.Args.Eq(cmp.Args) && me.Rets.Eq(cmp.Rets))
}

type GIrATypeRefStruct struct {
	Embeds    []string          `json:",omitempty"`
	Fields    GIrANamedTypeRefs `json:",omitempty"`
	PassByPtr bool              `json:",omitempty"`
}

func (me *GIrATypeRefStruct) Eq(cmp *GIrATypeRefStruct) bool {
	return (me == nil && cmp == nil) || (me != nil && cmp != nil && uslice.StrEq(me.Embeds, cmp.Embeds) && me.Fields.Eq(cmp.Fields))
}

type GonadIrAst struct {
	GIrABlock `json:",omitempty"`

	mod  *ModuleInfo
	proj *BowerProject
	girM *GonadIrMeta
}

type GIrA interface {
	subAsts() []GIrA
}

type gIrAConstable interface {
	GIrA
	isConstable() bool
}

type GIrAConst struct {
	GIrANamedTypeRef `json:",omitempty"`
	ConstVal         GIrA `json:",omitempty"`
}

func (me *GIrAConst) subAsts() []GIrA { return []GIrA{me.ConstVal} }

type GIrAVar struct {
	GIrANamedTypeRef `json:",omitempty"`
	VarVal           GIrA `json:",omitempty"`
}

func (me *GIrAVar) subAsts() []GIrA { return []GIrA{me.VarVal} }

type GIrAFunc struct {
	GIrANamedTypeRef `json:",omitempty"`
	FuncImpl         *GIrABlock `json:",omitempty"`
}

func (me *GIrAFunc) subAsts() []GIrA { return []GIrA{me.FuncImpl} }

type GIrALitStr struct {
	LitStr string
}

func (_ GIrALitStr) isConstable() bool { return true }
func (me *GIrALitStr) subAsts() []GIrA { return []GIrA{} }

type GIrALitBool struct {
	LitBool bool
}

func (_ GIrALitBool) isConstable() bool { return true }
func (me *GIrALitBool) subAsts() []GIrA { return []GIrA{} }

type GIrALitDouble struct {
	LitDouble float64
}

func (_ GIrALitDouble) isConstable() bool { return true }
func (me *GIrALitDouble) subAsts() []GIrA { return []GIrA{} }

type GIrALitInt struct {
	LitInt int
}

func (_ GIrALitInt) isConstable() bool { return true }
func (me *GIrALitInt) subAsts() []GIrA { return []GIrA{} }

type GIrABlock struct {
	Body []GIrA `json:",omitempty"`
}

func (me *GIrABlock) subAsts() []GIrA { return me.Body }

func (me *GIrABlock) Add(asts ...GIrA) {
	me.Body = append(me.Body, asts...)
}

type GIrAComments struct {
	Comments     []CoreImpComment `json:",omitempty"`
	CommentsDecl GIrA             `json:",omitempty"`
}

func (me *GIrAComments) subAsts() []GIrA { return []GIrA{me.CommentsDecl} }

type GIrAOp1 struct {
	Op1 string `json:",omitempty"`
	Of  GIrA   `json:",omitempty"`
}

func (me *GIrAOp1) subAsts() []GIrA { return []GIrA{me.Of} }

func (me GIrAOp1) isConstable() bool {
	if c, ok := me.Of.(gIrAConstable); ok {
		return c.isConstable()
	}
	return false
}

type GIrAOp2 struct {
	Left  GIrA   `json:",omitempty"`
	Op2   string `json:",omitempty"`
	Right GIrA   `json:",omitempty"`
}

func (me *GIrAOp2) subAsts() []GIrA { return []GIrA{me.Left, me.Right} }

func (me GIrAOp2) isConstable() bool {
	if c, ok := me.Left.(gIrAConstable); ok && c.isConstable() {
		if c, ok := me.Right.(gIrAConstable); ok {
			return c.isConstable()
		}
	}
	return false
}

type GIrASet struct {
	SetLeft GIrA `json:",omitempty"`
	ToRight GIrA `json:",omitempty"`
}

func (me *GIrASet) subAsts() []GIrA { return []GIrA{me.SetLeft, me.ToRight} }

type GIrAFor struct {
	ForDo    *GIrABlock `json:",omitempty"`
	ForCond  GIrA       `json:",omitempty"`
	ForInit  []*GIrASet `json:",omitempty"`
	ForStep  []*GIrASet `json:",omitempty"`
	ForRange *GIrAVar   `json:",omitempty"`
}

func (me *GIrAFor) subAsts() (all []GIrA) {
	all = append(all, me.ForDo, me.ForCond, me.ForRange)
	for _, fi := range me.ForInit {
		all = append(all, fi)
	}
	for _, fs := range me.ForStep {
		all = append(all, fs)
	}
	return
}

type GIrAIf struct {
	If   GIrA       `json:",omitempty"`
	Then *GIrABlock `json:",omitempty"`
	Else *GIrABlock `json:",omitempty"`
}

func (me *GIrAIf) subAsts() []GIrA { return []GIrA{me.If, me.Then, me.Else} }

type GIrACall struct {
	Callee   GIrA   `json:",omitempty"`
	CallArgs []GIrA `json:",omitempty"`
}

func (me *GIrACall) subAsts() []GIrA { return append(me.CallArgs, me.Callee) }

type GIrALitObj struct {
	GIrANamedTypeRef `json:",omitempty"`
	ObjFields        []*GIrALitObjField `json:",omitempty"`
}

func (me *GIrALitObj) subAsts() (all []GIrA) {
	for _, of := range me.ObjFields {
		all = append(all, of)
	}
	return
}

type GIrALitObjField struct {
	GIrANamedTypeRef `json:",omitempty"`
	FieldVal         GIrA `json:",omitempty"`
}

func (me *GIrALitObjField) subAsts() []GIrA { return []GIrA{me.FieldVal} }

type GIrANil struct {
	Nil interface{} // useless except we want to see it in the gonadast.json
}

func (me *GIrANil) subAsts() []GIrA { return []GIrA{} }

type GIrARet struct {
	RetArg GIrA `json:",omitempty"`
}

func (me *GIrARet) subAsts() []GIrA { return []GIrA{me.RetArg} }

type GIrAPanic struct {
	PanicArg GIrA `json:",omitempty"`
}

func (me *GIrAPanic) subAsts() []GIrA { return []GIrA{me.PanicArg} }

type GIrALitArr struct {
	GIrANamedTypeRef
	ArrVals []GIrA `json:",omitempty"`
}

func (me *GIrALitArr) subAsts() []GIrA { return me.ArrVals }

type GIrAIndex struct {
	IdxLeft  GIrA `json:",omitempty"`
	IdxRight GIrA `json:",omitempty"`
}

func (me *GIrAIndex) subAsts() []GIrA { return []GIrA{me.IdxLeft, me.IdxRight} }

type GIrADot struct {
	DotLeft  GIrA `json:",omitempty"`
	DotRight GIrA `json:",omitempty"`
}

func (me *GIrADot) subAsts() []GIrA { return []GIrA{me.DotLeft, me.DotRight} }

type GIrAIsType struct {
	ExprToTest GIrA `json:",omitempty"`
	TypeToTest GIrA `json:",omitempty"`
}

func (me *GIrAIsType) subAsts() []GIrA { return []GIrA{me.ExprToTest, me.TypeToTest} }

type GIrAToType struct {
	ExprToCast GIrA   `json:",omitempty"`
	TypePkg    string `json:",omitempty"`
	TypeName   string `json:",omitempty"`
}

func (me *GIrAToType) subAsts() []GIrA { return []GIrA{me.ExprToCast} }

type GIrAPkgRef struct {
	PkgName string `json:",omitempty"`
	Symbol  string `json:",omitempty"`
}

func (me *GIrAPkgRef) subAsts() []GIrA { return []GIrA{} }

func (me *GonadIrAst) FinalizePostPrep() (err error) {
	//	various fix-ups
	me.Walk(func(ast GIrA) GIrA {
		if ast != nil {
			switch a := ast.(type) {
			case *GIrAOp1:
				if a != nil && a.Op1 == "&" {
					//	restore data-ctors from calls like (&CtorName(1, '2', "3")) to turn into DataNameˇCtorName{1, '2', "3"}
					if oc, _ := a.Of.(*GIrACall); oc != nil {
						var gtd *GIrANamedTypeRef
						if ocd, _ := oc.Callee.(*GIrADot); ocd != nil {
							if ocd1, _ := ocd.DotLeft.(*GIrAVar); ocd1 != nil {
								if mod := FindModuleByPName(ocd1.NamePs); mod != nil {
									if ocd2, _ := ocd.DotRight.(*GIrAVar); ocd2 != nil {
										gtd = mod.girMeta.GoTypeDefByPsName(ocd.DotRight.(*GIrAVar).NamePs)
									}
								}
							}
						}
						ocv, _ := oc.Callee.(*GIrAVar)
						if gtd == nil && ocv != nil {
							gtd = me.girM.GoTypeDefByPsName(ocv.NameGo)
						}
						if gtd != nil {
							o := ªObj(gtd.NameGo)
							for _, ctorarg := range oc.CallArgs {
								o.ObjFields = append(o.ObjFields, &GIrALitObjField{FieldVal: ctorarg})
							}
							return o
						} else if ocv != nil && ocv.NamePs == "Error" {
							if !me.girM.Imports.Has("errors") {
								me.girM.Imports = append(me.girM.Imports, &GIrMPkgRef{used: true, N: "errors", P: "errors", Q: ""})
							}
							if len(oc.CallArgs) == 1 {
								if op2, _ := oc.CallArgs[0].(*GIrAOp2); op2 != nil && op2.Op2 == "+" {
									oc.CallArgs[0] = op2.Left
								}
							}
							return ªCall(ªPkgRef("errors", "New"), oc.CallArgs...)
						}
					}
				}
			}
		}
		return ast
	})

	//	link type-class-instance funcs to interface-implementing struct methods
	instfuncvars := me.topLevelDefs(func(a GIrA) bool {
		if v, _ := a.(*GIrAVar); v != nil {
			if vv, _ := v.VarVal.(*GIrALitObj); vv != nil {
				if gtd := me.girM.GoTypeDefByPsName(v.NamePs); gtd != nil {
					return true
				}
			}
		}
		return false
	})
	for _, ifx := range instfuncvars {
		ifv, _ := ifx.(*GIrAVar)
		if ifv == nil {
			ifv = ifx.(*GIrAComments).CommentsDecl.(*GIrAVar)
		}
		gtd := me.girM.GoTypeDefByPsName(ifv.NamePs) // the private implementer struct-type
		gtdInstOf := findGoTypeByPsQName(gtd.instOf)
		ifv.Export = gtdInstOf.Export
		ifv.setBothNamesFromPsName(ifv.NamePs)
		ifo := ifv.VarVal.(*GIrALitObj) //  something like:  InterfaceName{funcs}
		if strings.Contains(me.mod.srcFilePath, "TCls") {
			var tcctors []GIrA
			var mod *ModuleInfo
			pname, tcname := me.resolveGoTypeRef(gtd.instOf, true)
			if len(pname) == 0 || pname == me.mod.pName {
				mod = me.mod
			} else {
				mod = FindModuleByPName(pname)
			}
			tcctors = mod.girAst.topLevelDefs(func(a GIrA) bool {
				if fn, _ := a.(*GIrAFunc); fn != nil {
					return fn.WasTypeFunc && fn.NamePs == tcname
				}
				return false
			})
			if len(tcctors) > 0 {
				tcctor := tcctors[0].(*GIrAFunc)
				for i, instfuncarg := range tcctor.RefFunc.Args {
					for _, gtdmethod := range gtd.Methods {
						if gtdmethod.NamePs == instfuncarg.NamePs {
							ifofv := ifo.ObjFields[i].FieldVal
							switch ifa := ifofv.(type) {
							case *GIrAFunc:
								gtdmethod.method.body = ifa.FuncImpl
							default:
								gtdmethod.method.body = ªBlock(ªRet(ifofv))
							}
							break
						}
					}
				}
			}
			nuctor := ªObj(gtd.NameGo)
			nucast := ªTo(nuctor, pname, tcname)
			ifv.VarVal = nucast
		}
	}
	return
}

func (me *GonadIrAst) PrepFromCoreImp() (err error) {
	//	transform coreimp.json AST into our own leaner Go-focused AST format
	//	mostly focus on discovering new type-defs, final transforms once all
	//	type-defs in all modules are known happen in FinalizePostPrep
	for _, cia := range me.mod.coreimp.Body {
		me.Body = append(me.Body, cia.ciAstToGIrAst())
	}

	me.topLevelDefs(func(a GIrA) bool {
		if afn, _ := a.(*GIrAFunc); afn != nil {
			for _, gvd := range me.girM.GoValDecls {
				if gvd.NamePs == afn.NamePs {
					afn.Export = true
					afn.NameGo = gvd.NameGo
				}
			}
		}
		return false
	})

	//	detect unexported data-type constructors and add the missing structs implementing a new unexported single-per-pkg ADT interface type
	newxtypedatadecl := GIrMTypeDataDecl{Name: "ª" + me.mod.lName}
	var newextratypes GIrANamedTypeRefs
	var av *GIrAVar
	var ac *GIrAComments
	for i := 0; i < len(me.Body); i++ {
		if ac, _ = me.Body[i].(*GIrAComments); ac != nil && ac.CommentsDecl != nil {
			for tmp, _ := ac.CommentsDecl.(*GIrAComments); tmp != nil; tmp, _ = ac.CommentsDecl.(*GIrAComments) {
				ac = tmp
			}
			av, _ = ac.CommentsDecl.(*GIrAVar)
		} else {
			av, _ = me.Body[i].(*GIrAVar)
		}
		if av != nil && av.WasTypeFunc {
			if ac != nil {
				ac.CommentsDecl = nil
			}
			if fn, _ := av.VarVal.(*GIrAFunc); fn != nil {
				// TODO catches type-classes but not all
				// fmt.Printf("%v\t%s\t%s\t%s\n", len(fn.RefFunc.Args), av.NameGo, av.NamePs, me.mod.srcFilePath)
				// me.Body = append(me.Body[:i], me.Body[i+1:]...)
				// i--
			} else {
				fn := av.VarVal.(*GIrACall).Callee.(*GIrAFunc).FuncImpl.Body[0].(*GIrAFunc)
				if gtd := me.girM.GoTypeDefByPsName(av.NamePs); gtd == nil {
					nuctor := &GIrMTypeDataCtor{Name: av.NamePs, comment: ac}
					for i := 0; i < len(fn.RefFunc.Args); i++ {
						nuctor.Args = append(nuctor.Args, &GIrMTypeRef{TypeConstructor: "interface{/*PrepFromCoreImp*/}"})
					}
					newxtypedatadecl.Ctors = append(newxtypedatadecl.Ctors, nuctor)
				} else {
					gtd.comment = ac
				}
				me.Body = append(me.Body[:i], me.Body[i+1:]...)
				i--
			}
		}
	}
	if len(newxtypedatadecl.Ctors) > 0 {
		newextratypes = append(newextratypes, me.girM.toGIrADataTypeDefs([]GIrMTypeDataDecl{newxtypedatadecl}, map[string][]string{}, false)...)
	}
	//	also turn type-class instances into 0-byte structs providing the corresponding interface-implementing method(s)
	for _, tci := range me.girM.ExtTypeClassInsts {
		if gid := findGoTypeByPsQName(tci.ClassName); gid == nil {
			panic(me.mod.srcFilePath + ": type-class " + tci.ClassName + " not found for instance " + tci.Name)
		} else {
			gtd := newextratypes.ByPsName(tci.Name)
			if gtd == nil {
				gtd = &GIrANamedTypeRef{Export: true, instOf: tci.ClassName, RefStruct: &GIrATypeRefStruct{}}
				gtd.setBothNamesFromPsName(tci.Name)
				gtd.NameGo = "ı" + gtd.NameGo
				newextratypes = append(newextratypes, gtd)
			}
			for _, method := range gid.RefInterface.Methods {
				mcopy := *method
				mcopy.method.body = ªBlock(ªRet(nil))
				mcopy.method.hasNoThis = true
				gtd.Methods = append(gtd.Methods, &mcopy)
			}
		}
	}
	if len(newextratypes) > 0 {
		me.girM.GoTypeDefs = append(me.girM.GoTypeDefs, newextratypes...)
		me.girM.rebuildLookups()
	}

	//	now that we have these additional structs/interfaces, add private globals to represent all arg-less ctors
	nuglobals := []GIrA{}
	nuglobalsmap := map[string]string{}
	for _, gtd := range me.girM.GoTypeDefs {
		if gtd.RefInterface != nil && gtd.RefInterface.xtd != nil {
			for _, ctor := range gtd.RefInterface.xtd.Ctors {
				if ctor.gtd != nil && ctor.gtd != nil && len(ctor.Args) == 0 {
					nuvar := ªV("º" + ctor.Name)
					nuvar.VarVal = ªObj(ctor.gtd.NameGo)
					nuglobalsmap[ctor.Name] = nuvar.NameGo
					nuglobals = append(nuglobals, nuvar)
				}
			}
		}
	}
	me.Body = append(nuglobals, me.Body...)

	//	various fix-ups
	me.Walk(func(ast GIrA) GIrA {
		if ast != nil {
			switch a := ast.(type) {
			case *GIrADot:
				if dl, _ := a.DotLeft.(*GIrAVar); dl != nil {
					if dr, _ := a.DotRight.(*GIrAVar); dr != nil {
						//	find all CtorName.value references and change them to the above new vars
						if dr.NameGo == "value" {
							if nuglobalvarname, _ := nuglobalsmap[dl.NameGo]; len(nuglobalvarname) > 0 {
								return ªV(nuglobalvarname)
							}
						}
						//	if referring to a package, ensure the import is marked as in-use
						for _, imp := range me.girM.Imports {
							if imp.N == dl.NameGo {
								imp.used = true
								dr.Export = true
								dr.NameGo = sanitizeSymbolForGo(dr.NameGo, dr.Export)
								break
							}
						}
					}
				}
			case *GIrAVar:
				if a != nil {
					if vc, _ := a.VarVal.(gIrAConstable); vc != nil && vc.isConstable() {
						//	turn var=literal's into consts
						return ªConst(&a.GIrANamedTypeRef, a.VarVal)
					}
				}
			}
		}
		return ast
	})

	return
}

func (me *GonadIrAst) topLevelDefs(okay func(GIrA) bool) (defs []GIrA) {
	for _, ast := range me.Body {
		if okay(ast) {
			defs = append(defs, ast)
		} else if c, ok := ast.(*GIrAComments); ok {
			var c2 *GIrAComments
			for ok {
				if c2, ok = c.CommentsDecl.(*GIrAComments); ok {
					c = c2
				}
			}
			if okay(c.CommentsDecl) {
				defs = append(defs, ast)
			}
		}
	}
	return
}

func (me *GonadIrAst) Walk(on func(GIrA) GIrA) {
	for i, a := range me.Body {
		if a != nil {
			me.Body[i] = walk(a, on)
		}
	}
}

func (me *GonadIrAst) WriteAsJsonTo(w io.Writer) error {
	jsonenc := json.NewEncoder(w)
	jsonenc.SetIndent("", "\t")
	return jsonenc.Encode(me)
}

func (me *GonadIrAst) WriteAsGoTo(writer io.Writer) (err error) {
	var buf = &bytes.Buffer{}

	for _, gtd := range me.girM.GoTypeDefs {
		codeEmitTypeDecl(buf, gtd, 0, me.resolveGoTypeRef)
		if len(gtd.EnumConstNames) > 0 {
			enumtypename := toGIrAEnumTypeName(gtd.NamePs)
			codeEmitTypeAlias(buf, enumtypename, "int")
			codeEmitEnumConsts(buf, gtd.EnumConstNames, enumtypename)
		}
		codeEmitTypeMethods(buf, gtd, me.resolveGoTypeRef)
	}

	toplevelconsts := me.topLevelDefs(func(a GIrA) bool { _, ok := a.(*GIrAConst); return ok })
	toplevelvars := me.topLevelDefs(func(a GIrA) bool { _, ok := a.(*GIrAVar); return ok })

	codeEmitGroupedVals(buf, 0, true, toplevelconsts, me.resolveGoTypeRef)
	codeEmitGroupedVals(buf, 0, false, toplevelvars, me.resolveGoTypeRef)

	toplevelctorfuncs := me.topLevelDefs(func(a GIrA) bool { c, ok := a.(*GIrAVar); return ok && c.WasTypeFunc })
	toplevelfuncs := me.topLevelDefs(func(a GIrA) bool { c, ok := a.(*GIrAFunc); return ok && !c.WasTypeFunc })
	for _, ast := range toplevelctorfuncs {
		codeEmitAst(buf, 0, ast, me.resolveGoTypeRef)
		fmt.Fprint(buf, "\n\n")
	}
	for _, ast := range toplevelfuncs {
		codeEmitAst(buf, 0, ast, me.resolveGoTypeRef)
		fmt.Fprint(buf, "\n\n")
	}

	codeEmitPkgDecl(writer, me.mod.pName)
	sort.Sort(me.girM.Imports)
	codeEmitModImps(writer, me.girM.Imports)
	buf.WriteTo(writer)
	return
}

func (me *GonadIrAst) resolveGoTypeRef(tref string, markused bool) (pname string, tname string) {
	i := strings.LastIndex(tref, ".")
	if tname = tref[i+1:]; i > 0 {
		pname = tref[:i]
		if pname == me.mod.qName {
			pname = ""
		} else if pname == "Prim" {
			pname = ""
			switch tname {
			case "String":
				tname = "string"
			case "Boolean":
				tname = "bool"
			case "Number":
				tname = "float64"
			case "Int":
				tname = "int"
			default:
				panic("Unknown Prim type: " + tname)
			}
		} else {
			qn, foundimport, isffi := pname, false, strings.HasPrefix(pname, nsPrefixDefaultFfiPkg)
			var mod *ModuleInfo
			if isffi {
				pname = dot2underscore.Replace(pname)
			} else {
				if mod = FindModuleByQName(pname); mod == nil {
					panic(fmt.Errorf("%s: unknown module qname %s", me.mod.srcFilePath, qn))
				}
				pname = mod.pName
			}
			for _, imp := range me.girM.Imports {
				if imp.Q == qn {
					if foundimport = true; markused {
						imp.used = true
					}
					break
				}
			}
			if !foundimport {
				var imp *GIrMPkgRef
				if isffi {
					imp = &GIrMPkgRef{P: "github.com/metaleap/gonad/" + dot2slash.Replace(qn), Q: qn, N: pname}
				} else {
					imp = newModImp(mod)
				}
				if me.girM.imports, me.girM.Imports = append(me.girM.imports, mod), append(me.girM.Imports, imp); markused {
					imp.used = true
				}
			}
		}
	}
	return
}

func sanitizeSymbolForGo(name string, upper bool) string {
	if len(name) == 0 {
		return name
	}
	if upper {
		runes := []rune(name)
		runes[0] = unicode.ToUpper(runes[0])
		name = string(runes)
	} else {
		if ustr.BeginsUpper(name) {
			name = saniUpperToLowerPrefix + name
		} else {
			switch name {
			case "append", "false", "iota", "nil", "true":
				return "ˇ" + name
			case "break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for", "func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch", "type", "var":
				return "ˇĸˇ" + name
			}
		}
	}
	return sanitizer.Replace(name)
}

func walk(ast GIrA, on func(GIrA) GIrA) GIrA {
	if ast != nil {
		switch a := ast.(type) {
		case *GIrABlock:
			if a != nil { // odd that this would happen, given the above, but it did! (go1.7.6)
				for i, _ := range a.Body {
					a.Body[i] = walk(a.Body[i], on)
				}
			}
		case *GIrACall:
			a.Callee = walk(a.Callee, on)
			for i, _ := range a.CallArgs {
				a.CallArgs[i] = walk(a.CallArgs[i], on)
			}
		case *GIrAComments:
			a.CommentsDecl = walk(a.CommentsDecl, on)
		case *GIrAConst:
			a.ConstVal = walk(a.ConstVal, on)
		case *GIrADot:
			a.DotLeft, a.DotRight = walk(a.DotLeft, on), walk(a.DotRight, on)
		case *GIrAFor:
			a.ForCond = walk(a.ForCond, on)
			if tmp, ok := walk(a.ForRange, on).(*GIrAVar); ok && tmp != nil {
				a.ForRange = tmp
			}
			if tmp, ok := walk(a.ForDo, on).(*GIrABlock); ok && tmp != nil {
				a.ForDo = tmp
			}
			for i, fi := range a.ForInit {
				if tmp, ok := walk(fi, on).(*GIrASet); ok && tmp != nil {
					a.ForInit[i] = tmp
				}
			}
			for i, fs := range a.ForStep {
				if tmp, ok := walk(fs, on).(*GIrASet); ok && tmp != nil {
					a.ForStep[i] = tmp
				}
			}
		case *GIrAFunc:
			if tmp, ok := walk(a.FuncImpl, on).(*GIrABlock); ok && tmp != nil {
				a.FuncImpl = tmp
			}
		case *GIrAIf:
			a.If = walk(a.If, on)
			if tmp, ok := walk(a.Then, on).(*GIrABlock); ok && tmp != nil {
				a.Then = tmp
			}
			if tmp, ok := walk(a.Else, on).(*GIrABlock); ok && tmp != nil {
				a.Else = tmp
			}
		case *GIrAIndex:
			a.IdxLeft, a.IdxRight = walk(a.IdxLeft, on), walk(a.IdxRight, on)
		case *GIrAOp1:
			a.Of = walk(a.Of, on)
		case *GIrAOp2:
			a.Left, a.Right = walk(a.Left, on), walk(a.Right, on)
		case *GIrAPanic:
			a.PanicArg = walk(a.PanicArg, on)
		case *GIrARet:
			a.RetArg = walk(a.RetArg, on)
		case *GIrASet:
			a.SetLeft, a.ToRight = walk(a.SetLeft, on), walk(a.ToRight, on)
		case *GIrAVar:
			if a != nil { // odd that this would happen, given the above, but it did! (go1.7.6)
				a.VarVal = walk(a.VarVal, on)
			}
		case *GIrAIsType:
			a.ExprToTest, a.TypeToTest = walk(a.ExprToTest, on), walk(a.TypeToTest, on)
		case *GIrAToType:
			a.ExprToCast = walk(a.ExprToCast, on)
		case *GIrALitArr:
			for i, av := range a.ArrVals {
				a.ArrVals[i] = walk(av, on)
			}
		case *GIrALitObj:
			for i, av := range a.ObjFields {
				if tmp, ok := walk(av, on).(*GIrALitObjField); ok && tmp != nil {
					a.ObjFields[i] = tmp
				}
			}
		case *GIrALitObjField:
			a.FieldVal = walk(a.FieldVal, on)
		case *GIrAPkgRef, *GIrANil, *GIrALitBool, *GIrALitDouble, *GIrALitInt, *GIrALitStr:
		default:
			fmt.Printf("%v", ast)
			panic("WALK not handling a GIrA type")
		}
		ast = on(ast)
	}
	return ast
}

func ªCall(callee GIrA, callargs ...GIrA) *GIrACall {
	return &GIrACall{Callee: callee, CallArgs: callargs}
}

func ªConst(tref *GIrANamedTypeRef, val GIrA) *GIrAConst {
	return &GIrAConst{ConstVal: val, GIrANamedTypeRef: *tref}
}

func ªDot(left GIrA, right string) *GIrADot {
	return &GIrADot{DotLeft: left, DotRight: ªV(right)}
}

func ªEq(left GIrA, right GIrA) *GIrAOp2 {
	return &GIrAOp2{Op2: "==", Left: left, Right: right}
}

func ªO1(op string, operand GIrA) *GIrAOp1 {
	return &GIrAOp1{Op1: op, Of: operand}
}

func ªO2(left GIrA, op string, right GIrA) *GIrAOp2 {
	return &GIrAOp2{Op2: op, Left: left, Right: right}
}

func ªObj(typerefalias string) *GIrALitObj {
	return &GIrALitObj{GIrANamedTypeRef: GIrANamedTypeRef{RefAlias: typerefalias}}
}

func ªRet(retarg GIrA) *GIrARet {
	return &GIrARet{RetArg: retarg}
}

func ªBlock(asts ...GIrA) *GIrABlock {
	return &GIrABlock{Body: asts}
}

func ªB(literal bool) *GIrALitBool {
	return &GIrALitBool{LitBool: literal}
}

func ªPkgRef(pkgname string, symbol string) *GIrAPkgRef {
	return &GIrAPkgRef{PkgName: pkgname, Symbol: symbol}
}

func ªS(literal string) *GIrALitStr {
	return &GIrALitStr{LitStr: literal}
}

func ªSet(left string, right GIrA) *GIrASet {
	return &GIrASet{SetLeft: ªV(left), ToRight: right}
}

func ªV(name string) *GIrAVar {
	return &GIrAVar{GIrANamedTypeRef: GIrANamedTypeRef{NameGo: name}}
}

func ªTo(expr GIrA, pname string, tname string) *GIrAToType {
	return &GIrAToType{ExprToCast: expr, TypePkg: pname, TypeName: tname}
}
