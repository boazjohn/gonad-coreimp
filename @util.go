package main

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/metaleap/go-util/slice"
	"github.com/metaleap/go-util/str"
)

type never struct{}

const (
	msgfmt = "Encountered un-anticipated %s '%s' in %v,\n\tplease report the case with the *.purs code(base) so that I can support it, to: https://github.com/metaleap/gonad/issues."
)

var (
	strReplDot2Underscore = strings.NewReplacer(".", "_")
	strReplDot2Slash      = strings.NewReplacer(".", "/")
	strReplSlash2Dot      = strings.NewReplacer("\\", ".", "/", ".")
	strReplSanitizer      = strings.NewReplacer("'", "ˇ", "$", "Ø")
	strReplUnsanitize     = strings.NewReplacer("$prime", "'", "$$", "")
)

func notImplErr(cat string, name string, in interface{}) error {
	return fmt.Errorf(msgfmt, cat, name, in)
}

func panicWithType(in string, v interface{}, of string) {
	panic(fmt.Errorf("%s: unexpected value %v (type %v) for '%s'", in, v, reflect.TypeOf(v), of))
}

func ensureIfaceForTvar(tdict map[string][]string, tvar string, ifacetname string) {
	if ifaces4tvar := tdict[tvar]; !uslice.StrHas(ifaces4tvar, ifacetname) {
		ifaces4tvar = append(ifaces4tvar, ifacetname)
		tdict[tvar] = ifaces4tvar
	}
}

func findPsTypeByQName(qname string) (mod *modPkg, tr interface{}) {
	var pname, tname string
	i := strings.LastIndex(qname, ".")
	if tname = qname[i+1:]; i > 0 {
		pname = qname[:i]
		if mod = findModuleByQName(pname); mod == nil {
			panic(notImplErr("module qname", pname, qname))
		} else {
			for _, ets := range mod.irMeta.EnvTypeSyns {
				if ets.Name == tname {
					tr = ets
					return
				}
			}
			for _, etc := range mod.irMeta.EnvTypeClasses {
				if etc.Name == tname {
					tr = etc
					return
				}
			}
			for _, eti := range mod.irMeta.EnvTypeClassInsts {
				if eti.Name == tname {
					tr = eti
					return
				}
			}
			for _, etd := range mod.irMeta.EnvTypeDataDecls {
				if etd.Name == tname {
					tr = etd
					return
				}
			}
		}
	} else {
		panic(notImplErr("non-qualified type-name", qname, "a *.purs file of yours"))
	}
	return
}

func findGoTypeByGoQName(me *modPkg, qname string) (mod *modPkg, tref *irANamedTypeRef) {
	pname, tname := ustr.SplitOnce(qname, '.')
	if mod = findModuleByPName(pname); mod == nil {
		mod = me
	}
	tref = mod.irMeta.goTypeDefByGoName(tname)
	return
}

func findGoTypeByPsQName(qname string) (*modPkg, *irANamedTypeRef) {
	var pname, tname string
	i := strings.LastIndex(qname, ".")
	if tname = qname[i+1:]; i > 0 {
		pname = qname[:i]
		if mod := findModuleByQName(pname); mod == nil {
			panic(notImplErr("module qname", pname, qname))
		} else {
			return mod, mod.irMeta.goTypeDefByPsName(tname)
		}
	} else {
		panic(notImplErr("non-qualified type-name", qname, "a *.purs file of yours"))
	}
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
			runes := []rune(name)
			runes[0] = unicode.ToLower(runes[0])
			name = string(runes)
		} else {
			switch name {
			case "append", "false", "iota", "nil", "true":
				return "ˇ" + name + "ˇ"
			case "break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for", "func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch", "type", "var":
				return "ˇ" + name
			}
		}
	}
	return strReplSanitizer.Replace(name)
}

func typeNameWithPkgName(pkgname string, typename string) (fullname string) {
	if fullname = typename; len(pkgname) > 0 {
		fullname = pkgname + "." + fullname
	}
	return
}