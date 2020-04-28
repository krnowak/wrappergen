// Copyright Krzesimir Nowak
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

var isDbg = os.Getenv("DBG") == "1"

type aType struct {
	pkgName string
	name    string
}

func (at aType) String() string {
	if at.pkgName != "" {
		return fmt.Sprintf("%s.%s", at.pkgName, at.name)
	}
	return at.name
}

func (at aType) StringNoDot() string {
	if at.pkgName != "" {
		return fmt.Sprintf("%s%s", at.pkgName, at.name)
	}
	return at.name
}

func strToAType(s string) (aType, error) {
	if s == "" {
		return aType{}, fmt.Errorf("empty type string")
	}
	parts := strings.Split(s, ".")
	if len(parts) == 1 {
		return aType{
			pkgName: "",
			name:    s,
		}, nil
	} else if len(parts) == 2 {
		if parts[0] == "" {
			return aType{}, fmt.Errorf("empty package name in %s", s)
		}
		if parts[1] == "" {
			return aType{}, fmt.Errorf("empty type name in %s", s)
		}
		return aType{
			pkgName: parts[0],
			name:    parts[1],
		}, nil
	} else {
		return aType{}, fmt.Errorf("malformed type %s, expected a string like int or driver.Driver", s)
	}
}

type anImport struct {
	name string
	path string
}

func strToAnImport(s string) (anImport, error) {
	if s == "" {
		return anImport{}, fmt.Errorf("empty import string")
	}
	parts := strings.Split(s, ",")
	if len(parts) == 1 {
		return anImport{
			name: "",
			path: s,
		}, nil
	} else if len(parts) == 2 {
		if parts[0] == "" {
			return anImport{}, fmt.Errorf("empty import name in %s", s)
		}
		if parts[1] == "" {
			return anImport{}, fmt.Errorf("empty import path in %s", s)
		}
		return anImport{
			name: parts[0],
			path: parts[1],
		}, nil
	} else {
		return anImport{}, fmt.Errorf("malformed import string %s, expected either an import path or a comma-separated pair of a import name and import path", s)
	}
}

type extraField struct {
	name  string
	atype aType
}

func strToExtraField(s string) (extraField, error) {
	if s == "" {
		return extraField{}, fmt.Errorf("empty extra field string")
	}
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return extraField{}, fmt.Errorf("expected a comma-separated name-type pair for an extra field, got something else (%s)", s)
	}
	at, err := strToAType(parts[1])
	if err != nil {
		return extraField{}, fmt.Errorf("failed to get a type in extra field %s: %w", s, err)
	}
	return extraField{
		name:  parts[0],
		atype: at,
	}, nil
}

type resolvedType struct {
	at  aType
	pkg *packages.Package
	rt  *types.Named
}

func main() {
	inFileStr := flag.String("infile", "", "input file, if empty, GOFILE env var will be consulted")
	outFileStr := flag.String("outfile", "", "output file, if empty, will be deduced from the base type")
	baseTypeStr := flag.String("basetype", "", "base type, like driver.Conn")
	extTypesStr := flag.String("exttypes", "", "semicolon-separated list of extension types, like driver.ConnBeginTx,driver.ConnPrepareContext")
	extraFieldsStr := flag.String("extrafields", "", "semicolon-separated list of comma-separated pairs of names and types of extra fields, like count,int;rate,double")
	importsStr := flag.String("imports", "", "semicolon-separated list of imports; imports can be in form of either path (like database/sql/driver) or name,path (like driver,database/sql/driver)")
	prefix := flag.String("prefix", "", "prefix of the function called by interface implementations, like real (will cause Close method to call realClose function")
	newFunc := flag.String("newfunc", "", "name of the function creating a wrapper, like newConn")
	flag.Parse()

	if *baseTypeStr == "" {
		fail("no base type (or it is empty), use -basetype to specify it")
	}
	if *extTypesStr == "" {
		fail("no extension types, use -exttypes to specify them")
	}
	if *prefix == "" {
		fail("no prefix (or it is empty), use -prefix to specify it")
	}
	if *newFunc == "" {
		fail("no new func name (or it is empty), use -newfunc to specify it")
	}

	inFile := *inFileStr
	if inFile == "" {
		inFile = os.Getenv("GOFILE")
	}
	if inFile == "" {
		fail("no in file, use -infile to specify it or export the GOFILE environment variable")
	}
	{
		absInFile, err := filepath.Abs(inFile)
		if err != nil {
			fail("failed to get absolute path of infile %s: %v", inFile, err)
		}
		inFile = absInFile
	}
	inFileInfo, err := os.Stat(inFile)
	if err != nil {
		fail("failed to stat infile %s: %v", inFile, err)
	}
	if !inFileInfo.Mode().IsRegular() {
		fail("infile %s is not a file", inFile)
	}
	var baseType aType
	{
		var err error
		baseType, err = strToAType(*baseTypeStr)
		if err != nil {
			fail("failed to get a base type: %v", err)
		}
	}
	var extTypes []aType
	{
		ets := strings.Split(*extTypesStr, ";")
		for _, et := range ets {
			at, err := strToAType(et)
			if err != nil {
				fail("failed to get an extension type: %v", err)
			}
			extTypes = append(extTypes, at)
		}
	}
	var extraFields []extraField
	if len(*extraFieldsStr) > 0 {
		efs := strings.Split(*extraFieldsStr, ";")
		for _, ef := range efs {
			aef, err := strToExtraField(ef)
			if err != nil {
				fail("failed to get an extra field: %v", err)
			}
			extraFields = append(extraFields, aef)
		}
	}
	var imports []anImport
	if len(*importsStr) > 0 {
		is := strings.Split(*importsStr, ";")
		for _, i := range is {
			ai, err := strToAnImport(i)
			if err != nil {
				fail("failed to get an import: %v", err)
			}
			imports = append(imports, ai)
		}
	}
	pattern := fmt.Sprintf("file=%s", inFile)
	fset := token.NewFileSet()
	cfg := packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Fset:  fset,
		Tests: false,
	}
	pkgs, err := packages.Load(&cfg, pattern)
	if err != nil {
		fail("failed to load packages with pattern %s: %v", pattern, err)
	}
	if len(pkgs) != 1 {
		fail("loaded %d packages for pattern %s, expected one", len(pkgs), pattern)
	}
	thisPkg := pkgs[0]
	basePkgPath, err := getPkgPath(thisPkg, baseType, inFile, imports)
	if err != nil {
		fail("failed to get package path for base type %s: %v (means, the package of the base type is not imported in this package nor mentioned in -imports)", baseType, err)
	}
	basePkg, err := findPackage(&cfg, thisPkg, basePkgPath)
	if err != nil {
		fail("failed to find package %s for base type %s: %v (means, it isn't imported in this package, nor the go tools loader could load it", basePkgPath, baseType, err)
	}
	realBaseType, err := getType(basePkg, baseType.name)
	if err != nil {
		fail("failed to resolve the base type %s: %v (means, we could not find the type in the actual package)", baseType, err)
	}
	resolvedBaseType := resolvedType{
		at:  baseType,
		pkg: basePkg,
		rt:  realBaseType,
	}
	var resolvedExtTypes []resolvedType
	for _, extType := range extTypes {
		extPkgPath, err := getPkgPath(thisPkg, extType, inFile, imports)
		if err != nil {
			fail("failed to get package path for ext type %s: %v (means, the package of the ext type is not imported in this package nor mentioned in -imports)", extType, err)
		}
		extPkg, err := findPackage(&cfg, thisPkg, extPkgPath)
		if err != nil {
			fail("failed to find package %s for ext type %s: %v (means, it isn't imported in this package, nor the go tools loader could load it", extPkgPath, extType, err)
		}
		realExtType, err := getType(extPkg, extType.name)
		if err != nil {
			fail("failed to resolve the ext type %s: %v (means, we could not find the type in the actual package)", extType, err)
		}
		resolvedExtType := resolvedType{
			at:  extType,
			pkg: extPkg,
			rt:  realExtType,
		}
		resolvedExtTypes = append(resolvedExtTypes, resolvedExtType)
	}
	var resolvedEfTypes []resolvedType
	for _, ef := range extraFields {
		efPkgPath, err := getPkgPath(thisPkg, ef.atype, inFile, imports)
		if ef.atype.pkgName == "" {
			continue
		}
		if err != nil {
			fail("failed to get package path for extra field type %s: %v (means, the package of the extra field type is not imported in this package nor mentioned in -imports)", ef.atype, err)
		}
		efPkg, err := findPackage(&cfg, thisPkg, efPkgPath)
		if err != nil {
			fail("failed to find package %s for extra field type %s: %v (means, it isn't imported in this package, nor the go tools loader could load it", efPkgPath, ef.atype, err)
		}
		realEfType, err := getType(efPkg, ef.atype.name)
		if err != nil {
			fail("failed to resolve the extra field type %s: %v (means, we could not find the type in the actual package)", ef.atype, err)
		}
		resolvedEfType := resolvedType{
			at:  ef.atype,
			pkg: efPkg,
			rt:  realEfType,
		}
		resolvedEfTypes = append(resolvedEfTypes, resolvedEfType)
	}

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "// Code generated by \"wrappergen %s\"; DO NOT EDIT.\n", strings.Join(os.Args[1:], " "))
	fmt.Fprintf(buf, "\n")
	fmt.Fprintf(buf, "package %s\n", thisPkg.Name)
	fmt.Fprintf(buf, "\n")
	err = printImports(buf, thisPkg, resolvedBaseType, resolvedExtTypes, resolvedEfTypes, imports)
	if err != nil {
		fail("failed to print imports: %v", err)
	}
	fmt.Fprintf(buf, "\n")
	nComb := printTypes(buf, resolvedBaseType, resolvedExtTypes, extraFields)
	fmt.Fprintf(buf, "\n")
	printVars(buf, resolvedBaseType, resolvedExtTypes)
	fmt.Fprintf(buf, "\n")
	err = printImpls(buf, resolvedBaseType, resolvedExtTypes, *prefix, extraFields)
	if err != nil {
		fail("failed to print implementations: %v", err)
	}
	fmt.Fprintf(buf, "\n")
	printNewFunc(buf, *newFunc, *prefix, resolvedBaseType, resolvedExtTypes, nComb, extraFields)
	src, err := format.Source(buf.Bytes())
	if err != nil {
		warn("failed to format the code, compile to see what's wrong: %v", err)
		src = buf.Bytes()
	}
	outFile := *outFileStr
	if outFile == "" {
		baseName := fmt.Sprintf("%s_wrappers.go", resolvedBaseType.at.StringNoDot())
		outFile = filepath.Join(filepath.Dir(inFile), strings.ToLower(baseName))
	}
	err = ioutil.WriteFile(outFile, src, 0644)
	if err != nil {
		fail("failed to write source to outfile %s: %v", outFile, err)
	}
}

func printNewFunc(w io.Writer, funcName, prefix string, resolvedBaseType resolvedType, resolvedExtTypes []resolvedType, nComb int, extraFields []extraField) {
	varName := fmt.Sprintf("%s%s", prefix, resolvedBaseType.at.name)
	en := resolvedBaseType.at.StringNoDot()
	// exclude the zero - it will be handled after the switch
	fmt.Fprintf(w, "func %s(%s %s", funcName, varName, resolvedBaseType.at)
	for _, ef := range extraFields {
		fmt.Fprintf(w, ", %s %s", ef.name, ef.atype)
	}
	fmt.Fprintf(w, ") %s {\n\tswitch r := %s.(type) {\n", resolvedBaseType.at, varName)
	for counter := nComb - 1; counter > 0; counter-- {
		tbn := fmt.Sprintf("%s%d", en, counter)
		fmt.Fprintf(w, "\tcase i%s:\n\t\treturn &t%s{\n\t\t\tr: r,\n", tbn, tbn)
		for _, ef := range extraFields {
			fmt.Fprintf(w, "\t\t\t%s: %s,\n", ef.name, ef.name)
		}
		fmt.Fprintf(w, "\t\t}\n")
	}
	fmt.Fprintf(w, "\t}\nreturn &t%s0{\n\t\tr: %s,\n", en, varName)
	for _, ef := range extraFields {
		fmt.Fprintf(w, "\t\t%s: %s,\n", ef.name, ef.name)
	}
	fmt.Fprintf(w, "\t}\n}\n")
}

type parameter struct {
	name    string
	typeStr string
}

type parametersFull []parameter

func (p parametersFull) String() string {
	if len(p) == 0 {
		return ""
	}
	strs := make([]string, 0, len(p))
	for _, e := range p {
		strs = append(strs, fmt.Sprintf("%s %s", e.name, e.typeStr))
	}
	return strings.Join(strs, ", ")
}

type parametersNames []parameter

func (p parametersNames) String() string {
	if len(p) == 0 {
		return ""
	}
	strs := make([]string, 0, len(p))
	for _, e := range p {
		strs = append(strs, e.name)
	}
	return strings.Join(strs, ", ")
}

type parametersTypes []parameter

func (p parametersTypes) String() string {
	if len(p) == 0 {
		return ""
	}
	strs := make([]string, 0, len(p))
	for _, e := range p {
		strs = append(strs, e.typeStr)
	}
	return strings.Join(strs, ", ")
}

func printImpls(w io.Writer, resolvedBaseType resolvedType, resolvedExtTypes []resolvedType, prefix string, extraFields []extraField) error {
	comb := newCombinator(len(resolvedExtTypes))
	counter := 0
	en := resolvedBaseType.at.StringNoDot()
	first := true
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		if first {
			first = false
		} else {
			fmt.Fprintf(w, "\n")
		}
		handled, err := printImplsFromResolvedType(w, resolvedBaseType, tbn, prefix, extraFields, nil)
		if err != nil {
			return err
		}
		for _, idx := range idxs {
			handled, err = printImplsFromResolvedType(w, resolvedExtTypes[idx], tbn, prefix, extraFields, handled)
			if err != nil {
				return err
			}
		}
		counter++
	}
	return nil
}

type stringSet map[string]struct{}

func (s stringSet) Add(str string) {
	s[str] = struct{}{}
}

func (s stringSet) AddSet(other stringSet) {
	for str := range other {
		s.Add(str)
	}
}

func (s stringSet) Has(str string) bool {
	_, ok := s[str]
	return ok
}

func printExplicitImplsOfInterface(w io.Writer, iface *types.Interface, tbn, prefix string, extraFields []extraField) error {
	for idx := 0; idx < iface.NumExplicitMethods(); idx++ {
		m := iface.ExplicitMethod(idx)
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return fmt.Errorf("function %s has no signature", m.Name())
		}
		params, err := tupleToParameters(sig.Params())
		if err != nil {
			return err
		}
		results, err := tupleToParameters(sig.Results())
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "func (o%s *t%s) %s(%s)", tbn, tbn, m.Name(), (parametersFull)(params))
		switch len(results) {
		case 0:
			// nothing to print
		case 1:
			fmt.Fprintf(w, " %s", (parametersTypes)(results))
		default:
			fmt.Fprintf(w, " (%s)", (parametersTypes)(results))
		}
		fmt.Fprintf(w, " {\n\t")
		if len(results) > 0 {
			fmt.Fprintf(w, "return ")
		}
		fmt.Fprintf(w, "%s%s(o%s.r", prefix, m.Name(), tbn)
		for _, ef := range extraFields {
			fmt.Fprintf(w, ", o%s.%s", tbn, ef.name)
		}
		if len(params) > 0 {
			fmt.Fprintf(w, ", %s", (parametersNames)(params))
		}
		fmt.Fprintf(w, ")\n}\n")
	}
	return nil
}

func printImplsOfEmbeddedTypes(w io.Writer, iface *types.Interface, excludes stringSet, tbn, prefix string, extraFields []extraField) (stringSet, error) {
	newExcludes := stringSet{}
	for idx := 0; idx < iface.NumEmbeddeds(); idx++ {
		et := iface.EmbeddedType(idx)
		named, ok := et.(*types.Named)
		if !ok {
			return nil, fmt.Errorf("embedded type %s is not an named type (%#v)", et, et)
		}
		obj := named.Obj()
		if excludes.Has(obj.Id()) {
			continue
		}
		newExcludes.Add(obj.Id())
		underType := named.Underlying()
		underIface, ok := underType.(*types.Interface)
		if !ok {
			return nil, fmt.Errorf("embedded type %s is not a named interface type (%#v)", obj.Name(), underType)
		}
		subExcludes, err := printImplsFromInterfaceRecursive(w, underIface, newExcludes, tbn, prefix, extraFields)
		if err != nil {
			return nil, fmt.Errorf("failed to print impls of embedded named interface %s: %w", obj.Name(), err)
		}
		newExcludes.AddSet(subExcludes)
	}
	return newExcludes, nil
}

func printImplsFromInterfaceRecursive(w io.Writer, iface *types.Interface, excludes stringSet, tbn, prefix string, extraFields []extraField) (stringSet, error) {
	subExcludes, err := printImplsOfEmbeddedTypes(w, iface, excludes, tbn, prefix, extraFields)
	if err != nil {
		return nil, err
	}
	err = printExplicitImplsOfInterface(w, iface, tbn, prefix, extraFields)
	if err != nil {
		return nil, err
	}
	newExcludes := stringSet{}
	newExcludes.AddSet(excludes)
	newExcludes.AddSet(subExcludes)
	return newExcludes, nil
}

func printImplsFromResolvedType(w io.Writer, resType resolvedType, tbn, prefix string, extraFields []extraField, excludes stringSet) (stringSet, error) {
	underType := resType.rt.Underlying()
	underIface, ok := underType.(*types.Interface)
	if !ok {
		return nil, fmt.Errorf("%s is not an interface", resType.at)
	}
	newExcludes := stringSet{}
	newExcludes.AddSet(excludes)
	newExcludes.Add(resType.rt.Obj().Id())
	subExcludes, err := printImplsFromInterfaceRecursive(w, underIface, newExcludes, tbn, prefix, extraFields)
	if err != nil {
		return nil, fmt.Errorf("failed to print impls for interface %s: %w", resType.at, err)
	}
	return subExcludes, nil
}

func typeToStr(vType types.Type) string {
	switch vRealType := vType.(type) {
	case *types.Named:
		vNamedTypeObj := vRealType.Obj()
		vPkg := vNamedTypeObj.Pkg()
		if vPkg != nil {
			return fmt.Sprintf("%s.%s", vPkg.Name(), vNamedTypeObj.Name())
		}
		return vNamedTypeObj.Name()
	case *types.Basic:
		return vRealType.Name()
	case *types.Slice:
		elemStr := typeToStr(vRealType.Elem())
		if elemStr == "" {
			return ""
		}
		return fmt.Sprintf("[]%s", elemStr)
	case *types.Pointer:
		elemStr := typeToStr(vRealType.Elem())
		if elemStr == "" {
			return ""
		}
		return fmt.Sprintf("*%s", elemStr)
	default:
		return ""
	}
}

func tupleToParameters(t *types.Tuple) ([]parameter, error) {
	if t == nil || t.Len() == 0 {
		return nil, nil
	}
	var simpleParams []parameter
	for idx := 0; idx < t.Len(); idx++ {
		v := t.At(idx)
		vName := v.Name()
		if vName == "" {
			vName = fmt.Sprintf("param%d", idx)
		}
		vType := v.Type()
		vTypeStr := typeToStr(vType)
		if vTypeStr == "" {
			debug("parameter %s is %s (%#v)", vName, vType, vType)
			return nil, fmt.Errorf("could not handle parameter %s (%T)", vName, vType)
		}
		simpleParams = append(simpleParams, parameter{
			name:    vName,
			typeStr: vTypeStr,
		})
	}
	return simpleParams, nil
}

func printVars(w io.Writer, resolvedBaseType resolvedType, resolvedExtTypes []resolvedType) {
	fmt.Fprintf(w, "var (\n")
	counter := 0
	en := resolvedBaseType.at.StringNoDot()
	comb := newCombinator(len(resolvedExtTypes))
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		fmt.Fprintf(w, "\t_ %s = &t%s{}\n", resolvedBaseType.at, tbn)
		for _, idx := range idxs {
			fmt.Fprintf(w, "\t_ %s = &t%s{}\n", resolvedExtTypes[idx].at, tbn)
		}
		counter++
	}
	fmt.Fprintf(w, ")\n")
}

type combinator struct {
	n   int
	idx []int
}

func newCombinator(n int) *combinator {
	return &combinator{
		n:   n,
		idx: nil,
	}
}

func (c *combinator) Next() bool {
	if len(c.idx) > c.n {
		return false
	}
	if c.idx == nil {
		c.idx = []int{}
		return true
	}
	i := len(c.idx) - 1
	l := c.n - 1
	for i >= 0 {
		if c.idx[i] < l {
			c.idx[i]++
			return true
		}
		i--
		l--
	}
	c.idx = append(c.idx, 0)
	if len(c.idx) > c.n {
		return false
	}
	for i := range c.idx {
		c.idx[i] = i
	}
	return true
}

func (c *combinator) Get() []int {
	return c.idx
}

func printTypes(w io.Writer, resolvedBaseType resolvedType, resolvedExtTypes []resolvedType, extraFields []extraField) int {
	fmt.Fprintf(w, "type (\n")
	counter := 0
	en := resolvedBaseType.at.StringNoDot()
	comb := newCombinator(len(resolvedExtTypes))
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		fmt.Fprintf(w, "\n\ti%s interface {\n\t\t%s\n", tbn, resolvedBaseType.at)
		for _, idx := range idxs {
			fmt.Fprintf(w, "\t\t%s\n", resolvedExtTypes[idx].at)
		}
		fmt.Fprintf(w, "\t}\n\n\tt%s struct {\n\t\tr i%s\n", tbn, tbn)
		for _, ef := range extraFields {
			fmt.Fprintf(w, "\t\t%s %s", ef.name, ef.atype)
		}
		fmt.Fprintf(w, "\t}\n")
		counter++
	}
	fmt.Fprintf(w, ")\n")
	return counter
}

func printImports(w io.Writer, thisPkg *packages.Package, resolvedBaseType resolvedType, resolvedExtTypes, resolvedEfTypes []resolvedType, imports []anImport) error {
	type namedPkg struct {
		pkg  *packages.Package
		name string
	}
	dedupPackages := map[string]namedPkg{}
	allResolvedTypes := make([]resolvedType, 0, 1+len(resolvedExtTypes)+len(resolvedEfTypes))
	allResolvedTypes = append(allResolvedTypes, resolvedBaseType)
	allResolvedTypes = append(allResolvedTypes, resolvedExtTypes...)
	allResolvedTypes = append(allResolvedTypes, resolvedEfTypes...)
	for _, ret := range allResolvedTypes {
		np, ok := dedupPackages[ret.pkg.ID]
		if ok {
			if np.name == "" || np.name == ret.at.pkgName {
				continue
			}
			if ret.pkg.Name != ret.at.pkgName {
				return fmt.Errorf("inconsistent imported package name, package %s is referred as %s and as %s, either fix the name in -imports or -basetype or in infile's imports", ret.pkg.Name, np.name, ret.at.pkgName)
			}
			np.name = ""
		} else {
			np.pkg = ret.pkg
			if resolvedBaseType.pkg.Name != resolvedBaseType.at.pkgName {
				np.name = resolvedBaseType.at.pkgName
			} else {
				np.name = ""
			}
		}
		dedupPackages[ret.pkg.ID] = np
	}

	fmt.Fprintf(w, "import (\n")
	for _, imprt := range imports {
		if imprt.name != "" {
			fmt.Fprintf(w, "\t%s %q\n", imprt.name, imprt.path)
		} else {
			fmt.Fprintf(w, "\t%q\n", imprt.path)
		}
	}
	for id, np := range dedupPackages {
		if id == thisPkg.ID {
			continue
		}
		if np.name != "" {
			fmt.Fprintf(w, "\t%s %q\n", np.name, np.pkg.PkgPath)
		} else {
			fmt.Fprintf(w, "\t%q\n", np.pkg.PkgPath)
		}
	}
	fmt.Fprintf(w, ")\n")
	return nil
}

func getPkgPath(thisPkg *packages.Package, at aType, inFile string, imports []anImport) (string, error) {
	for _, imprt := range imports {
		if imprt.name == at.pkgName {
			return imprt.path, nil
		}
	}
	idx := -1
	for i, path := range thisPkg.CompiledGoFiles {
		if path == inFile {
			idx = i
			break
		}
	}
	if idx >= 0 && len(thisPkg.Syntax) > idx {
		f := thisPkg.Syntax[idx]
		for _, imprt := range f.Imports {
			if imprt.Name != nil && imprt.Name.Name == at.pkgName {
				return imprt.Path.Value, nil
			}
		}
	}
	for path, ipkg := range thisPkg.Imports {
		if ipkg.Name == at.pkgName {
			return path, nil
		}
	}
	return "", fmt.Errorf("package path for %s not found", at.pkgName)
}

func findPackage(cfg *packages.Config, thisPkg *packages.Package, pkgPath string) (*packages.Package, error) {
	if pkg := findPackageNoLoad(thisPkg, pkgPath); pkg != nil {
		return pkg, nil
	}
	// still not found, load it
	loadedPkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s package: %w", pkgPath, err)
	}
	for _, lpkg := range loadedPkgs {
		if pkg := findPackageNoLoad(lpkg, pkgPath); pkg != nil {
			return pkg, nil
		}
	}
	return nil, fmt.Errorf("package %s not found", pkgPath)
}

func findPackageNoLoad(fpkg *packages.Package, pkgPath string) *packages.Package {
	pkgsToGo := []*packages.Package{fpkg}
	for i := 0; i < len(pkgsToGo); i++ {
		pkg := pkgsToGo[i]
		if pkg.PkgPath == pkgPath {
			return pkg
		}
		for _, ipkg := range pkg.Imports {
			pkgsToGo = append(pkgsToGo, ipkg)
		}
	}
	return nil
}

func getType(pkg *packages.Package, name string) (*types.Named, error) {
	debug("searching for type %s in package %s (%s)", name, pkg.Name, pkg.PkgPath)
	obj := pkg.Types.Scope().Lookup(name)
	if obj != nil {
		named, ok := obj.Type().(*types.Named)
		if ok {
			debug("found a type through lookup: %s", named)
			return named, nil
		}
	}
	return nil, fmt.Errorf("no type %s in pkg %s", name, pkg.Name)
}

func fail(formatStr string, args ...interface{}) {
	printWithPrefix("ERROR", formatStr, args...)
	os.Exit(1)
}

func warn(formatStr string, args ...interface{}) {
	printWithPrefix("WARN", formatStr, args...)
}

/*
func info(formatStr string, args ...interface{}) {
	printWithPrefix("INFO", formatStr, args...)
}
*/

func debug(formatStr string, args ...interface{}) {
	if !isDbg {
		return
	}
	printWithPrefix("DEBUG", formatStr, args...)
}

func printWithPrefix(prefix, formatStr string, args ...interface{}) {
	newFormatStr := fmt.Sprintf("%s: %s\n", prefix, formatStr)
	fmt.Fprintf(os.Stderr, newFormatStr, args...)
}
