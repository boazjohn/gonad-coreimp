package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

type goTypeRefResolver func(tref string) (pname string, tname string)

func codeEmitCoreImp(w io.Writer, indent int, ast *CoreImpAst) {
	tabs := strings.Repeat("\t", indent)
	switch ast.Ast_tag {
	case "StringLiteral":
		fmt.Fprintf(w, "%q", ast.StringLiteral)
	case "BooleanLiteral":
		fmt.Fprintf(w, "%t", ast.BooleanLiteral)
	case "NumericLiteral_Double":
		fmt.Fprintf(w, "%f", ast.NumericLiteral_Double)
	case "NumericLiteral_Integer":
		fmt.Fprintf(w, "%d", ast.NumericLiteral_Integer)
	case "Var":
		fmt.Fprintf(w, "%s", ast.Var)
	case "Block":
		fmt.Fprint(w, "{\n")
		indent++
		for _, expr := range ast.Block {
			codeEmitCoreImp(w, indent, expr)
		}
		fmt.Fprintf(w, "%s}", tabs)
		indent--
	case "While":
		fmt.Fprintf(w, "%sfor ", tabs)
		codeEmitCoreImp(w, indent, ast.While)
		codeEmitCoreImp(w, indent, ast.Ast_body)
	case "For":
		fmt.Fprintf(w, "%sfor %s ; ", tabs, ast.For)
		codeEmitCoreImp(w, indent, ast.Ast_for1)
		fmt.Fprint(w, " ; ")
		codeEmitCoreImp(w, indent, ast.Ast_for2)
		fmt.Fprint(w, " ")
		codeEmitCoreImp(w, indent, ast.Ast_body)
	case "ForIn":
		fmt.Fprintf(w, "%sfor _, %s := range ", tabs, ast.ForIn)
		codeEmitCoreImp(w, indent, ast.Ast_for1)
		codeEmitCoreImp(w, indent, ast.Ast_body)
	case "IfElse":
		fmt.Fprintf(w, "%sif ", tabs)
		codeEmitCoreImp(w, indent, ast.IfElse)
		fmt.Fprint(w, " ")
		codeEmitCoreImp(w, indent, ast.Ast_ifThen)
		if ast.Ast_ifElse != nil {
			fmt.Fprint(w, " else ")
			codeEmitCoreImp(w, indent, ast.Ast_ifElse)
		}
		fmt.Fprint(w, "\n")
	case "App":
		codeEmitCoreImp(w, indent, ast.App)
		fmt.Fprint(w, "(")
		for i, expr := range ast.Ast_appArgs {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			codeEmitCoreImp(w, indent, expr)
		}
		fmt.Fprint(w, ")")
	case "Function":
		fmt.Fprintf(w, "func %s(", ast.Function)
		for i, argname := range ast.Ast_funcParams {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprint(w, argname)
		}
		fmt.Fprint(w, ") ")
		codeEmitCoreImp(w, indent, ast.Ast_body)
	case "Unary":
		fmt.Fprint(w, "(")
		switch ast.Ast_op {
		case "Negate":
			fmt.Fprint(w, "-")
		case "Not":
			fmt.Fprint(w, "!")
		case "Positive":
			fmt.Fprint(w, "+")
		case "BitwiseNot":
			fmt.Fprint(w, "^")
		default:
			fmt.Fprintf(w, "?%s?", ast.Ast_op)
			panic("unrecognized unary op '" + ast.Ast_op + "', please report!")
		}
		codeEmitCoreImp(w, indent, ast.Unary)
		fmt.Fprint(w, ")")
	case "Binary":
		fmt.Fprint(w, "(")
		codeEmitCoreImp(w, indent, ast.Binary)
		switch ast.Ast_op {
		case "Add":
			fmt.Fprint(w, "+")
		case "Subtract":
			fmt.Fprint(w, "-")
		case "Multiply":
			fmt.Fprint(w, "*")
		case "Divide":
			fmt.Fprint(w, "/")
		case "Modulus":
			fmt.Fprint(w, "%")
		case "EqualTo":
			fmt.Fprint(w, "==")
		case "NotEqualTo":
			fmt.Fprint(w, "!=")
		case "LessThan":
			fmt.Fprint(w, "<")
		case "LessThanOrEqualTo":
			fmt.Fprint(w, "<=")
		case "GreaterThan":
			fmt.Fprint(w, ">")
		case "GreaterThanOrEqualTo":
			fmt.Fprint(w, ">=")
		case "And":
			fmt.Fprint(w, "&&")
		case "Or":
			fmt.Fprint(w, "||")
		case "BitwiseAnd":
			fmt.Fprint(w, "&")
		case "BitwiseOr":
			fmt.Fprint(w, "|")
		case "BitwiseXor":
			fmt.Fprint(w, "^")
		case "ShiftLeft":
			fmt.Fprint(w, "<<")
		case "ShiftRight":
			fmt.Fprint(w, ">>")
		case "ZeroFillShiftRight":
			fmt.Fprint(w, "&^")
		default:
			fmt.Fprintf(w, "?%s?", ast.Ast_op)
			panic("unrecognized binary op '" + ast.Ast_op + "', please report!")
		}
		codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
		fmt.Fprint(w, ")")
	case "VariableIntroduction":
		fmt.Fprintf(w, "%svar %s", tabs, ast.VariableIntroduction)
		if ast.Ast_rightHandSide != nil {
			fmt.Fprint(w, " = ")
			codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
		}
		fmt.Fprint(w, "\n")
	case "Comment":
		for _, c := range ast.Comment {
			if c != nil {
				if len(c.BlockComment) > 0 {
					fmt.Fprintf(w, "/*%s*/", c.BlockComment)
				} else {
					fmt.Fprintf(w, "%s//%s\n", tabs, c.LineComment)
				}
			}
		}
		if ast.Ast_decl != nil {
			codeEmitCoreImp(w, indent, ast.Ast_decl)
		}
	case "ObjectLiteral":
		fmt.Fprint(w, "{")
		for i, namevaluepair := range ast.ObjectLiteral {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			for onekey, oneval := range namevaluepair {
				fmt.Fprintf(w, "%s: ", onekey)
				codeEmitCoreImp(w, indent, oneval)
				break
			}
		}
		fmt.Fprint(w, "}")
	case "ReturnNoResult":
		fmt.Fprintf(w, "%sreturn\n", tabs)
	case "Return":
		fmt.Fprintf(w, "%sreturn ", tabs)
		codeEmitCoreImp(w, indent, ast.Return)
		fmt.Fprint(w, "\n")
	case "Throw":
		fmt.Fprintf(w, "%spanic(", tabs)
		codeEmitCoreImp(w, indent, ast.Throw)
		fmt.Fprint(w, ")\n")
	case "ArrayLiteral":
		fmt.Fprint(w, "[]ARRAY{")
		for i, expr := range ast.ArrayLiteral {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			codeEmitCoreImp(w, indent, expr)
		}
		fmt.Fprint(w, "}")
	case "Assignment":
		fmt.Fprint(w, tabs)
		codeEmitCoreImp(w, indent, ast.Assignment)
		fmt.Fprint(w, " = ")
		codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
		fmt.Fprint(w, "\n")
	case "Indexer":
		codeEmitCoreImp(w, indent, ast.Indexer)
		if ast.Ast_rightHandSide.Ast_tag == "StringLiteral" {
			fmt.Fprintf(w, ".%s", ast.Ast_rightHandSide.StringLiteral)
			// codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
		} else {
			fmt.Fprint(w, "[")
			codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
			fmt.Fprint(w, "]")
		}
	case "InstanceOf":
		codeEmitCoreImp(w, indent, ast.InstanceOf)
		fmt.Fprint(w, " is ")
		codeEmitCoreImp(w, indent, ast.Ast_rightHandSide)
	default:
		panic("CoreImp unhandled AST-tag, please report: " + ast.Ast_tag)
	}
}

func codeEmitCoreImps(w io.Writer, indent int, body []*CoreImpAst) {
	for _, ast := range body {
		codeEmitCoreImp(w, indent, ast)
	}
}

func codeEmitEnumConsts(buf io.Writer, enumconstnames []string, enumconsttype string) {
	fmt.Fprint(buf, "const (\n")
	for i, enumconstname := range enumconstnames {
		fmt.Fprintf(buf, "\t%s", enumconstname)
		if i == 0 {
			fmt.Fprintf(buf, " %s = iota", enumconsttype)
		}
		fmt.Fprint(buf, "\n")
	}
	fmt.Fprint(buf, ")\n")
}

func codeEmitFuncArgs(w io.Writer, methodargs GIrANamedTypeRefs, typerefresolver goTypeRefResolver) {
	if len(methodargs) > 0 {
		fmt.Fprint(w, "(")
		for i, arg := range methodargs {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			if len(arg.Name) > 0 {
				fmt.Fprintf(w, "%s ", arg.Name)
			}
			codeEmitTypeDecl(w, arg, false, typerefresolver)
		}
		fmt.Fprint(w, ") ")
	}
}

func codeEmitModImps(writer io.Writer, modimps []*GIrMPkgRef) {
	if len(modimps) > 0 {
		fmt.Fprint(writer, "import (\n")
		for _, modimp := range modimps {
			if modimp.used {
				fmt.Fprintf(writer, "\t%s %q\n", modimp.N, modimp.P)
			} else {
				fmt.Fprintf(writer, "\t// %s %q\n", modimp.N, modimp.P)
			}
		}
		fmt.Fprint(writer, ")\n\n")
	}
}

func codeEmitPkgDecl(writer io.Writer, pname string) {
	fmt.Fprintf(writer, "package %s\n\n", pname)
}

func codeEmitTypeAlias(buf io.Writer, tname string, ttype string) {
	fmt.Fprintf(buf, "type %s %s\n", tname, ttype)
}

func codeEmitTypeDecl(w io.Writer, gtd *GIrANamedTypeRef, toplevel bool, typerefresolver goTypeRefResolver) {
	fmtembeds := "%s; "
	if toplevel {
		fmtembeds = "\t%s\n"
		fmt.Fprintf(w, "type %s ", gtd.Name)
	}
	if len(gtd.RefAlias) > 0 {
		fmt.Fprint(w, codeEmitTypeRef(typerefresolver(gtd.RefAlias)))
	} else if gtd.RefUnknown > 0 {
		fmt.Fprintf(w, "interface{/*%d*/}", gtd.RefUnknown)
	} else if gtd.RefInterface != nil {
		fmt.Fprint(w, "interface {")
		if toplevel {
			fmt.Fprintln(w)
		}
		for _, ifaceembed := range gtd.RefInterface.Embeds {
			fmt.Fprintf(w, fmtembeds, codeEmitTypeRef(typerefresolver(ifaceembed)))
		}
		fmt.Fprint(w, "}")
	} else if gtd.RefStruct != nil {
		fmt.Fprint(w, "struct {")
		if toplevel {
			fmt.Fprintln(w)
		}
		for _, structembed := range gtd.RefStruct.Embeds {
			fmt.Fprintf(w, fmtembeds, structembed)
		}
		for _, structfield := range gtd.RefStruct.Fields {
			var buf bytes.Buffer
			codeEmitTypeDecl(&buf, structfield, false, typerefresolver)
			fmt.Fprintf(w, fmtembeds, structfield.Name+" "+buf.String())
		}
		fmt.Fprint(w, "}")
	} else if gtd.RefFunc != nil {
		fmt.Fprint(w, "func (")
		for i, l := 0, len(gtd.RefFunc.Args); i < l; i++ {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			if argname := gtd.RefFunc.Args[i].Name; len(argname) > 0 {
				fmt.Fprintf(w, "%s ", argname)
			}
			codeEmitTypeDecl(w, gtd.RefFunc.Args[i], false, typerefresolver)
		}
		fmt.Fprint(w, ") (")
		for i, l := 0, len(gtd.RefFunc.Rets); i < l; i++ {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			if retname := gtd.RefFunc.Rets[i].Name; len(retname) > 0 {
				fmt.Fprintf(w, "%s ", retname)
			}
			codeEmitTypeDecl(w, gtd.RefFunc.Rets[i], false, typerefresolver)
		}
		fmt.Fprint(w, ")")
	}
	if toplevel {
		fmt.Fprintln(w)
	}
}

func codeEmitTypeMethods(w io.Writer, tr *GIrANamedTypeRef, typerefresolver goTypeRefResolver) {
	for _, meth := range tr.Methods {
		fmt.Fprintf(w, "func (me *%s) %s ", tr.Name, meth.Name)
		codeEmitFuncArgs(w, meth.RefFunc.Args, typerefresolver)
		codeEmitFuncArgs(w, meth.RefFunc.Args, typerefresolver)
		codeEmitFuncArgs(w, meth.RefFunc.Rets, typerefresolver)
		fmt.Fprint(w, "{\n")
		codeEmitCoreImps(w, 1, meth.methodBody)
		fmt.Fprint(w, "}\n")
	}
}

func codeEmitTypeRef(pname string, tname string) string {
	if len(pname) == 0 {
		return tname
	}
	return pname + "." + tname
}