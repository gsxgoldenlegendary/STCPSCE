package main

import (
	"bytes"
	"container/list"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
)

type Ast struct {
	Label    string            `json:"label"`
	Pos      int               `json:"pos"`
	End      int               `json:"end"`
	Attrs    map[string]string `json:"attrs"`
	Children []*Ast            `json:"children"`
}

// This is a type used to represent the result of the parsing.
// Used in debugging.

//type Result struct {
//	*Ast `json:"ast"`
//	//Source string `json:"source"`
//	//Dump   string `json:"dump"`
//}

// This is used to determine if a node is a basic label.
// Now I choose `Ident` or `SelectorExpr` as basic labels.
func isBasicLabel(ast *Ast) bool {
	if strings.Contains(ast.Label, "Fun") {
		return false
	} else if strings.Contains(ast.Label, "Key") {
		return false
	} else if strings.Contains(ast.Label, "Type") {
		return false
	} else if strings.Contains(ast.Label, "*ast.Ident") {
		return true
	} else if strings.Contains(ast.Label, "*ast.SelectorExpr") {
		return true
	}
	return false
}

// This is used to determine if two nodes are equal.
// If two nodes are both basic labels, then compare their names.
// If two nodes are both non-basic labels, then compare their children recursively.
func astNodeEqual(ast1 *Ast, ast2 *Ast) bool {
	if strings.Contains(ast1.Label, "Name") && strings.Contains(ast2.Label, "Name") {
		if ast1.Attrs["Name"] == ast2.Attrs["Name"] {
			return true
		}
	} else if len(ast1.Children) == len(ast2.Children) {
		for x := range ast1.Children {
			if !astNodeEqual(ast1.Children[x], ast2.Children[x]) {
				return false
			}
		}
		return true
	}
	return false
}

// This is used to find the labels in condition statements.
func addLabelsInConditionStatement(ast *Ast) (labels *list.List) {
	labels = list.New()
	for x := range ast.Children {
		if isBasicLabel(ast.Children[x]) {
			labels.PushBack(ast.Children[x])
		} else {
			labels.PushBackList(addLabelsInConditionStatement(ast.Children[x]))
		}
	}
	return labels
}

// This is used to check if the labels in condition statements are in the left-handed side of assignment statements.
func checkLabelsInAssignStatementLeftHandedSide(ast *Ast, labels *list.List) bool {
	for x := range ast.Children {
		for e := labels.Front(); e != nil; e = e.Next() {
			if astNodeEqual(ast.Children[x], e.Value.(*Ast)) {
				return true
			}
		}
		//Theoretically, we should check the labels in the left-handed side of assignment statements recursively.
		//But in practice, we only need to check the first level of the left-handed side of assignment statements.
		//if checkLabelsInAssignStatementLeftHandedSide(ast.Children[x], labels) {
		//	return true
		//}
	}
	return false
}

// This is used to check if the labels in the right-handed side of assignment statements are in the left-handed side of assignment statements.
func checkLabelsInAssignStatementRightHandedSide(ast *Ast, functionArguments []*Ast, labels *list.List) bool {
	for x := range ast.Children {
		// no need to consider `BasicLit`
		if strings.Contains(ast.Children[x].Label, "BasicLit") {
			return false
		} else if strings.Contains(ast.Children[x].Label, "*ast.Ident") {
			for y := range functionArguments {
				if astNodeEqual(ast.Children[x], functionArguments[y]) {
					return false
				}
			}
			for e := labels.Front(); e != nil; e = e.Next() {
				if astNodeEqual(ast.Children[x], e.Value.(*Ast)) {
					return false
				}
			}
			// need to investigate the arguments of function calls
		} else if strings.Contains(ast.Children[x].Label, "CallExpr") {
			for z := range ast.Children[x].Children[1].Children {
				for y := range functionArguments {
					if astNodeEqual(ast.Children[x].Children[1].Children[z], functionArguments[y]) {
						return false
					}
				}
				for e := labels.Front(); e != nil; e = e.Next() {
					if astNodeEqual(ast.Children[x].Children[1].Children[z], e.Value.(*Ast)) {
						return false
					}
				}
				return true
			}
		}
		//Theoretically, we should check the labels in the right-handed side of assignment statements recursively.
		//But in practice, we only need to check the first level of the right-handed side of assignment statements.
		//if !checkLabelsInAssignStatementRightHandedSide(ast.Children[x], functionArguments) {
		//	return false
		//}
	}
	return true
}

// This is used to find the labels in half statements, including right-handed side and left-handed side of assignment statements.
func findLabelsInHalfStatements(ast *Ast) (labels *list.List) {
	labels = list.New()
	for x := range ast.Children {
		if strings.Contains(ast.Children[x].Label, "CallExpr") {
			labels.PushBackList(findLabelsInHalfStatements(ast.Children[x].Children[1]))
		} else if isBasicLabel(ast.Children[x]) {
			labels.PushBack(ast.Children[x])
		} else {
			labels.PushBackList(findLabelsInHalfStatements(ast.Children[x]))
		}
	}
	return labels
}

// This is used to trim the repeated labels.
func trimList(labels *list.List) {
	for x := labels.Front(); x != nil; x = x.Next() {
		for y := x.Next(); y != nil; y = y.Next() {
			if astNodeEqual(x.Value.(*Ast), y.Value.(*Ast)) {
				labels.Remove(y)
			}
		}
	}
}

// This is used to find all statements relative to the exchangeable sentences.
// It is like expanding the kernels.
func expendKernels(ast *Ast, kernels []*Ast) (pos []*Ast) {
	pos = []*Ast{}
	for kernel := range kernels {
		var x int
		// Step 1: find the last statement which can be parallelized.
		for x = len(ast.Children) - 1; x >= 0; x-- {
			if astNodeEqual(ast.Children[x], kernels[kernel]) {
				break
			}
		}
		tempLabels := list.New()
		tempLabels.PushBackList(findLabelsInHalfStatements(ast.Children[x].Children[1]))
		trimList(tempLabels)
		pos = append(pos, ast.Children[x])
		// Step 2: find the statements which can be parallelized before the last statement.
		for x--; tempLabels.Len() != 0 && x >= 0; x-- {
			if strings.Contains(ast.Children[x].Label, "AssignStmt") {
				flag := false
				for z := range ast.Children[x].Children[0].Children {
					for e := tempLabels.Front(); e != nil; e = e.Next() {
						if astNodeEqual(ast.Children[x].Children[0].Children[z], e.Value.(*Ast)) {
							tempLabels.Remove(e)
							flag = true
						}
					}
				}
				// If flag is true, it means that some new labels are added in the label list.
				if flag {
					tempLabels.PushBackList(findLabelsInHalfStatements(ast.Children[x].Children[1]))
					trimList(tempLabels)
					pos = append(pos, ast.Children[x])
				}
			}
		}
	}
	return pos
}

// This is used to find all exchangeable sentences in the function declaration.
func analyzeFunctionDeclaration(ast *Ast) (posList *list.List) {
	posList = list.New()
	if strings.Contains(ast.Label, "FuncDecl") {
		var arguments []*Ast
		// Step 1: find the arguments of the function.
		for x := range ast.Children[2].Children[0].Children[0].Children {
			arguments = append(arguments, ast.Children[2].Children[0].Children[0].Children[x].Children[0].Children[0])
		}
		// Step 2: find the exchangeable sentences in the function.
		kernels := findExchangeableSentences(ast, arguments)
		// Step 3: expand the kernels.
		if len(kernels) != 0 {
			posList.PushBack(expendKernels(ast.Children[3].Children[0], kernels))
		}
	} else {
		// The `else` part is used to link each list of exchangeable sentences in different functions.
		for x := range ast.Children {
			posList.PushBackList(analyzeFunctionDeclaration(ast.Children[x]))
		}
	}
	return posList
}

// This is used to find the labels in the left-handed side of assignment statements.
func addLabelsInLeftValue(ast *Ast) (labels *list.List) {
	labels = list.New()
	for x := range ast.Children {
		if isBasicLabel(ast.Children[x]) {
			labels.PushBack(ast.Children[x])
		} else {
			labels.PushBackList(addLabelsInLeftValue(ast.Children[x]))
		}
	}
	return labels
}

// This is used to find the exchangeable sentences in the function.
func findExchangeableSentences(ast *Ast, functionArguments []*Ast) (pos []*Ast) {
	pos = []*Ast{}
	if strings.Contains(ast.Label, "List : []ast.Stmt") {
		labelsInCondition := list.New()
		labelsInLeftHandedSide := list.New()
		for x := range ast.Children {
			// If the statement is `IfStmt`, then we need to find the labels in the condition statement.
			if strings.Contains(ast.Children[x].Label, "IfStmt") {
				labelsInCondition.PushBackList(addLabelsInConditionStatement(ast.Children[x]))
				// If the statement is `IncDecStmt` and the self-increasing or self-decreasing label is not in the
				//conditions which in front of it, it means that the statement can be parallelized.
			} else if strings.Contains(ast.Children[x].Label, "IncDecStmt") {
				for e := labelsInCondition.Front(); e != nil; e = e.Next() {
					if astNodeEqual(ast.Children[x].Children[0], e.Value.(*Ast)) {
						goto A
					}
				}
				pos = append(pos, ast.Children[x])
				// If the statement is `AssignStmt`, then we need to check if the operator is `:=`.
				// If the operator is `:=`, then we need to find the labels in the left-handed side of assignment statements.
				// If the operator is `=`, then we need to check if the labels in the left-handed side of assignment statements
				// are in the conditions which in front of it and if the labels in the right-handed side of assignment statements
				// are in the left-handed side of assignment statements.
			} else if strings.Contains(ast.Children[x].Label, "AssignStmt") {
				if ast.Children[x].Attrs["Tok"] == ":=" {
					labelsInLeftHandedSide.PushBackList(addLabelsInLeftValue(ast.Children[x].Children[0]))
				} else {
					if !checkLabelsInAssignStatementLeftHandedSide(ast.Children[x].Children[0],
						labelsInCondition) && !checkLabelsInAssignStatementRightHandedSide(ast.Children[x].
						Children[1], functionArguments, labelsInLeftHandedSide) {
						pos = append(pos, ast.Children[x])
					}
				}
			}
		A: //It is my coding style to use `goto` to break the nested loop.
		}
	} else {
		for x := range ast.Children {
			pos = append(pos, findExchangeableSentences(ast.Children[x], functionArguments)...)
		}
	}
	return pos
}

// This is used to find `GetState` or `PutState` expressions in the function.
func findGetOrPutStateExpression(ast *Ast, GetStateMap map[string][]int, isGet bool) (ArgumentPosition []int) {
	ArgumentPosition = []int{}
	if strings.Contains(ast.Label, "CallExpr") {
		if strings.Contains(ast.Children[0].Label, "SelectorExpr") {
			if isGet {
				if ast.Children[0].Children[1].Attrs["Name"] == "GetState" {
					ArgumentPosition = []int{0}
				}
			} else {
				if ast.Children[0].Children[1].Attrs["Name"] == "PutState" {
					ArgumentPosition = []int{0}
				}
			}
		} else {
			ArgumentPosition = GetStateMap[ast.Children[0].Attrs["Name"]]
		}
	}
	for x := range ast.Children {
		ArgumentPosition = append(ArgumentPosition, findGetOrPutStateExpression(ast.Children[x], GetStateMap, isGet)...)
	}
	return ArgumentPosition
}

func findGetOrPutStateList(ast *Ast, GetStateMap map[string][]int, arguments []*Ast, isGet bool) (GetStateList []int) {
	GetStateList = []int{}
	var argumentsPosition []int
	tempLabels := list.New()
	for x := len(ast.Children) - 1; x >= 0; x-- {
		argumentsPosition = findGetOrPutStateExpression(ast.Children[x], GetStateMap, isGet)
		if len(argumentsPosition) != 0 {
			for y := range argumentsPosition {
				tempLabels.PushBack(ast.Children[x].Children[len(ast.Children[x].Children)-1].Children[0].Children[1].Children[argumentsPosition[y]])
			}
			trimList(tempLabels)
		} else if strings.Contains(ast.Children[x].Label, "AssignStmt") {
			flag := false
			for z := range ast.Children[x].Children[0].Children {
				for e := tempLabels.Front(); e != nil; e = e.Next() {
					if astNodeEqual(ast.Children[x].Children[0].Children[z], e.Value.(*Ast)) {
						tempLabels.Remove(e)
						flag = true
					}
				}
			}
			if flag {
				tempLabels.PushBackList(findLabelsInHalfStatements(ast.Children[x].Children[len(ast.Children[x].Children)-1]))
				trimList(tempLabels)
			}
		}
	}
	// If the label is `SelectorExpr` or `IndexExpr`, then we need to use the labels before the operator `.` or `[`.
	for e := tempLabels.Front(); e != nil; e = e.Next() {
		if strings.Contains(e.Value.(*Ast).Label, "SelectorExpr") || strings.Contains(e.Value.(*Ast).Label, "IndexExpr") {
			tempLabels.PushBack(e.Value.(*Ast).Children[0])
			tempLabels.Remove(e)
		}
	}
	// It must be trimmed again because the labels before the operator `.` or `[` may be repeated.
	trimList(tempLabels)
	// I haven't found a better way to find the position of the labels in the arguments.
	for x := range arguments {
		for e := tempLabels.Front(); e != nil; e = e.Next() {
			if astNodeEqual(arguments[x], e.Value.(*Ast)) {
				GetStateList = append(GetStateList, x)
			}
		}
	}
	return GetStateList
}

func analyzeReadWriteAPI(ast *Ast) (GetStateMap map[string][]int, PutStateMap map[string][]int) {
	GetStateMap = make(map[string][]int)
	PutStateMap = make(map[string][]int)
	for flag := true; flag; {
		flag = false
		for y := range ast.Children {
			if strings.Contains(ast.Children[y].Label, "FuncDecl") {
				var arguments []*Ast
				for x := range ast.Children[y].Children[len(ast.Children[y].Children)-2].Children[0].Children[0].Children {
					arguments = append(arguments, ast.Children[y].Children[len(ast.Children[y].Children)-2].Children[0].Children[0].Children[x].Children[0].Children[0])
				}
				// Here is a complicated logic. I will explain it in detail.
				// The basic idea is update the `GetStateMap` and `PutStateMap` until they are not changed.
				// So we need to find the new `GetStateMap` and `PutStateMap` in each iteration.
				// Then use a `DeepEqual` function to check if the `GetStateMap` and `PutStateMap` are changed.
				// The code `[len(ast.Children[y].Children)-3]` is used to process some nodes which lack of some children.
				if !reflect.DeepEqual(GetStateMap[ast.Children[y].Children[len(ast.Children[y].Children)-3].
					Attrs["Name"]], findGetOrPutStateList(ast.Children[y].Children[len(ast.Children[y].Children)-1].
					Children[0], GetStateMap, arguments, true)) {
					GetStateMap[ast.Children[y].Children[len(ast.Children[y].Children)-3].
						Attrs["Name"]] = findGetOrPutStateList(ast.
						Children[y].Children[len(ast.Children[y].Children)-1].Children[0], GetStateMap, arguments, true)
					flag = true
				}
				if !reflect.DeepEqual(PutStateMap[ast.Children[y].Children[len(ast.Children[y].Children)-3].
					Attrs["Name"]], findGetOrPutStateList(ast.Children[y].Children[len(ast.Children[y].Children)-1].
					Children[0], GetStateMap, arguments, false)) {
					PutStateMap[ast.Children[y].Children[len(ast.Children[y].Children)-3].
						Attrs["Name"]] = findGetOrPutStateList(ast.
						Children[y].Children[len(ast.Children[y].Children)-1].Children[0], GetStateMap, arguments, false)
					flag = true
				}
			}
		}
	}
	return GetStateMap, PutStateMap
}

func Parse(filename string, source string) (err error) {

	// Create the AST by parsing src.
	fileSet := token.NewFileSet() // positions are relative to fileSet
	f, err := parser.ParseFile(fileSet, filename, source, parser.ParseComments)

	a, err := BuildAst("", f)
	if err != nil {
		return err
	}

	posList := analyzeFunctionDeclaration(a)
	fmt.Print("Phase 1:\n")
	for pos := posList.Front(); pos != nil; pos = pos.Next() {
		fmt.Print("[")
		for x := range pos.Value.([]*Ast) {
			fmt.Print(fileSet.File(f.Pos()).Line(fileSet.File(f.Pos()).Pos(pos.Value.([]*Ast)[x].Pos)))
			fmt.Print(", ")
		}
		fmt.Print("\b\b]\n")
	}
	fmt.Print("\nPhase2: Read/Write API:\n")
	GetStateList, PutStateList := analyzeReadWriteAPI(a.Children[1])
	fmt.Print("GetState:\n")
	fmt.Print(GetStateList)
	fmt.Print("\nPutState:\n")
	fmt.Print(PutStateList)
	//body, err := json.Marshal(Result{Ast: a})
	//if err != nil {
	//	return err
	//}
	//err = ioutil.WriteFile("ast.json", body, 0666)
	//if err != nil {
	//	return err
	//}

	return nil
}

func BuildAst(prefix string, n interface{}) (astObj *Ast, err error) {
	v := reflect.ValueOf(n)
	t := v.Type()

	a := Ast{Label: Label(prefix, n), Attrs: map[string]string{}, Children: []*Ast{}}

	if node, ok := n.(ast.Node); ok {
		a.Pos = int(node.Pos())
		a.End = int(node.End())
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if v.IsValid() == false {
		return nil, nil
	}

	switch v.Kind() {
	case reflect.Array, reflect.Slice:

		for i := 0; i < v.Len(); i++ {
			f := v.Index(i)

			child, err := BuildAst(fmt.Sprintf("%d", i), f.Interface())
			if err != nil {
				return nil, err
			}
			a.Children = append(a.Children, child)
		}
	case reflect.Map:
		for _, kv := range v.MapKeys() {
			f := v.MapIndex(kv)

			child, err := BuildAst(fmt.Sprintf("%v", kv.Interface()), f.Interface())
			if err != nil {
				return nil, err
			}
			a.Children = append(a.Children, child)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			fo := f
			name := t.Field(i).Name

			if f.Kind() == reflect.Ptr {
				f = f.Elem()
			}

			if f.IsValid() == false {
				continue
			}

			if _, ok := v.Interface().(ast.Object); !ok && f.Kind() == reflect.Interface {

				switch f.Interface().(type) {
				case ast.Decl, ast.Expr, ast.Node, ast.Spec, ast.Stmt:

					child, err := BuildAst(name, f.Interface())
					if err != nil {
						return nil, err
					}
					a.Children = append(a.Children, child)
					continue
				}
			}

			switch f.Kind() {
			case reflect.Struct, reflect.Array, reflect.Slice, reflect.Map:
				child, err := BuildAst(name, fo.Interface())
				if err != nil {
					return nil, err
				}
				a.Children = append(a.Children, child)

			default:
				a.Attrs[name] = fmt.Sprintf("%v", f.Interface())
			}
		}
	}

	return &a, nil
}

func Label(prefix string, n interface{}) string {

	var bf bytes.Buffer
	var err error
	if prefix != "" {
		_, err = fmt.Fprintf(&bf, "%s : ", prefix)
	}
	_, err = fmt.Fprintf(&bf, "%T", n)
	if err != nil {
		fmt.Println(err)
	}

	v := reflect.ValueOf(n)
	t := v.Type()

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	if v.IsValid() == false {
		return ""
	}

	switch v.Kind() {

	case reflect.Array, reflect.Slice, reflect.Map, reflect.Chan:
		_, err = fmt.Fprintf(&bf, "(len = %d)", v.Len())

	case reflect.Struct:
		if v.Kind() == reflect.Struct {
			var fs []string
			for i := 0; i < v.NumField(); i++ {
				f := v.Field(i)
				name := t.Field(i).Name
				switch name {
				case "Name", "Kind", "Tok", "Op":
					fs = append(fs, fmt.Sprintf("%s: %v", name, f.Interface()))
				}
			}
			if len(fs) > 0 {
				_, err = fmt.Fprintf(&bf, " (%s)", strings.Join(fs, ", "))
			}
		}
	default:
		_, err = fmt.Fprintf(&bf, " : %s", n)
	}
	return string(bf.Bytes())
}

func main() {
	inputFile := ""
	if len(os.Args) == 2 {
		inputFile = os.Args[1]
	} else {
		fmt.Println("Example: go run main.go input.txt")
		return
	}
	src, err := ioutil.ReadFile(inputFile)
	source := string(src)
	err = Parse("foo", source)
	if err != nil {
		fmt.Println("Error", err)
	}
}
