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
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/types"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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
	name    string
	typeStr string
	expr    ast.Expr
}

func strToExtraField(s string) (extraField, error) {
	if s == "" {
		return extraField{}, fmt.Errorf("empty extra field string")
	}
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return extraField{}, fmt.Errorf("expected a comma-separated name-type pair for an extra field, got something else (%s)", s)
	}
	expr, err := parser.ParseExpr(parts[1])
	if err != nil {
		return extraField{}, fmt.Errorf("failed to get an AST for extra field %s (likely invalid Go snippet in type part): %w", s, err)
	}
	return extraField{
		name:    parts[0],
		typeStr: parts[1],
		expr:    expr,
	}, nil
}

type resolvedType struct {
	at          aType
	rt          *types.Named
	origPkgName string // empty for builtin types
	pkgPath     string // empty for builtin types
}

type silentFailureType struct{}

var (
	silentFailure silentFailureType
	_             error = silentFailure
)

func (silentFailureType) Error() string {
	return ""
}

func main() {
	if err := mainErr(); err != nil {
		if err != silentFailure {
			printWithPrefix("ERROR", "%v", err)
		}
		os.Exit(1)
	}
}

func mainErr() error {
	flagset := flag.NewFlagSet("wrappergen", flag.ContinueOnError)
	fi := &flagsInput{}
	fi.configureFlagSet(flagset)
	if err := fi.parseFlagsAndEnvironment(flagset, os.Args[1:], os.Environ()); err != nil {
		return err
	}
	if err := fi.ensureValid(); err != nil {
		return err
	}
	pi := &parsedInput{}
	if err := pi.parseInput(fi); err != nil {
		return err
	}
	fi = nil // we don't need it any more
	rt := &resolvedTypes{}
	if err := rt.resolveTypes(pi); err != nil {
		return err
	}
	ta := &typeAnalysis{}
	if err := ta.analyze(rt, pi.imports); err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "// Code generated by \"wrappergen %s\"; DO NOT EDIT.\n", strings.Join(os.Args[1:], " "))
	fmt.Fprintf(buf, "\n")
	fmt.Fprintf(buf, "package %s\n", rt.thisPkgName)
	fmt.Fprintf(buf, "\n")
	printImports(buf, ta)
	fmt.Fprintf(buf, "\n")
	printTypes(buf, rt, pi.extraFields)
	fmt.Fprintf(buf, "\n")
	printVars(buf, rt)
	fmt.Fprintf(buf, "\n")
	printImpls(buf, rt, ta, pi.prefix, pi.extraFields)
	fmt.Fprintf(buf, "\n")
	printNewFunc(buf, pi.newFuncName, pi.prefix, rt, pi.extraFields)
	src, err := format.Source(buf.Bytes())
	if err != nil {
		warn("failed to format the code, compile to see what's wrong: %v", err)
		src = buf.Bytes()
	}
	err = ioutil.WriteFile(pi.outFile, src, 0644)
	if err != nil {
		return fmt.Errorf("failed to write source to outfile %s: %w", pi.outFile, err)
	}
	return nil
}

type flagsInput struct {
	inFile      string
	outFile     string
	baseType    string
	extTypes    string
	extraFields string
	imports     string
	prefix      string
	newFuncName string
}

func (fi *flagsInput) configureFlagSet(flagset *flag.FlagSet) {
	flagset.StringVar(&fi.inFile, "infile", "", "input file, if empty, GOFILE env var will be consulted")
	flagset.StringVar(&fi.outFile, "outfile", "", "output file, if empty, will be deduced from the base type")
	flagset.StringVar(&fi.baseType, "basetype", "", "base type, like driver.Conn")
	flagset.StringVar(&fi.extTypes, "exttypes", "", "semicolon-separated list of extension types, like driver.ConnBeginTx,driver.ConnPrepareContext")
	flagset.StringVar(&fi.extraFields, "extrafields", "", "semicolon-separated list of comma-separated pairs of names and types of extra fields, like count,int;rate,double")
	flagset.StringVar(&fi.imports, "imports", "", "semicolon-separated list of imports; imports can be in form of either path (like database/sql/driver) or name,path (like driver,database/sql/driver)")
	flagset.StringVar(&fi.prefix, "prefix", "", "prefix of the function called by interface implementations, like real (will cause Close method to call realClose function")
	flagset.StringVar(&fi.newFuncName, "newfuncname", "", "name of the function creating a wrapper, like newConn")
}

func (fi *flagsInput) parseFlagsAndEnvironment(flagset *flag.FlagSet, args, environ []string) error {
	if err := flagset.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return silentFailure
		}
		return err
	}
	if fi.inFile == "" {
		for _, envkv := range environ {
			if strings.HasPrefix(envkv, "GOFILE=") {
				fi.inFile = envkv[7:]
				break
			}
		}
	}
	return nil
}

func (fi *flagsInput) ensureValid() error {
	if fi.baseType == "" {
		return errors.New("no base type (or it is empty), use -basetype to specify it")
	}
	if fi.prefix == "" {
		return errors.New("no prefix (or it is empty), use -prefix to specify it")
	}
	if fi.newFuncName == "" {
		return errors.New("no new func name (or it is empty), use -newfuncname to specify it")
	}
	if fi.inFile == "" {
		return errors.New("no in file, use -infile to specify it or export the GOFILE environment variable")
	}
	inFileInfo, err := os.Stat(fi.inFile)
	if err != nil {
		return fmt.Errorf("failed to stat infile %s: %w", fi.inFile, err)
	}
	if !inFileInfo.Mode().IsRegular() {
		return fmt.Errorf("infile %s is not a file", fi.inFile)
	}
	return nil
}

type parsedInput struct {
	baseType    aType
	extTypes    []aType
	extraFields []extraField
	imports     []anImport
	inFile      string
	outFile     string
	prefix      string
	newFuncName string
}

func (pi *parsedInput) parseInput(fi *flagsInput) error {
	{
		baseType, err := strToAType(fi.baseType)
		if err != nil {
			return fmt.Errorf("failed to get base type from input parameter %s: %w", fi.baseType, err)
		}
		pi.baseType = baseType
	}
	if fi.extTypes != "" {
		ets := strings.Split(fi.extTypes, ";")
		for _, et := range ets {
			at, err := strToAType(et)
			if err != nil {
				return fmt.Errorf("failed to get an extension type from input parameter %s: %w", et, err)
			}
			pi.extTypes = append(pi.extTypes, at)
		}
	}
	if fi.extraFields != "" {
		efs := strings.Split(fi.extraFields, ";")
		for _, ef := range efs {
			aef, err := strToExtraField(ef)
			if err != nil {
				return fmt.Errorf("failed to get an extra field from input parameter %s: %w", ef, err)
			}
			pi.extraFields = append(pi.extraFields, aef)
		}
	}
	if fi.imports != "" {
		is := strings.Split(fi.imports, ";")
		for _, i := range is {
			ai, err := strToAnImport(i)
			if err != nil {
				return fmt.Errorf("failed to get an import from input parameter %s: %w", i, err)
			}
			pi.imports = append(pi.imports, ai)
		}
	}
	if filepath.IsAbs(fi.inFile) {
		pi.inFile = fi.inFile
	} else if absPath, err := filepath.Abs(fi.inFile); err != nil {
		return fmt.Errorf("failed to get an absolute path of the infile %s: %w", fi.inFile, err)
	} else {
		pi.inFile = absPath
	}
	if fi.outFile != "" {
		pi.outFile = fi.outFile
	} else {
		baseName := fmt.Sprintf("%s_wrappers.go", pi.baseType.StringNoDot())
		pi.outFile = filepath.Join(filepath.Dir(pi.inFile), strings.ToLower(baseName))
	}
	if !isValidFunctionName(fi.prefix) {
		return fmt.Errorf("prefix %s is invalid, it should start with either uppercase or lowercase ASCII character or an underline, and then followed by uppercase or lowercase ASCII characters or ASCII digits or underlines", fi.prefix)
	}
	pi.prefix = fi.prefix
	if !isValidFunctionName(fi.newFuncName) {
		return fmt.Errorf("function name %s is invalid, it should start with either uppercase or lowercase ASCII character or an underline, and then followed by uppercase or lowercase ASCII characters or ASCII digits or underlines", fi.newFuncName)
	}
	pi.newFuncName = fi.newFuncName
	return nil
}

func isValidFunctionName(s string) bool {
	if s == "" {
		return false
	}
	if (s[0] < 'A' || s[0] > 'Z') &&
		(s[0] < 'a' || s[0] > 'z') &&
		(s[0] != '_') {
		return false
	}
	for idx := 1; idx < len(s); idx++ { // first character was already checked
		if (s[idx] < 'A' || s[idx] > 'Z') &&
			(s[idx] < 'a' || s[idx] > 'z') &&
			(s[idx] < '0' || s[idx] > '9') &&
			(s[idx] != '_') {
			return false
		}
	}
	return true
}

type resolvedTypes struct {
	thisPkgName      string
	thisPkgPath      string
	resolvedBaseType resolvedType
	resolvedExtTypes []resolvedType
	resolvedEfTypes  []resolvedType
}

func (rt *resolvedTypes) resolveTypes(pi *parsedInput) error {
	pattern := fmt.Sprintf("file=%s", pi.inFile)
	cfg := packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps | packages.NeedTypes,
		Logf: debug,
		// TODO: specify parser function that skips function
		// bodies
	}
	pkgs, err := packages.Load(&cfg, pattern)
	if err != nil {
		return fmt.Errorf("failed to load packages with pattern %s: %w", pattern, err)
	}
	if len(pkgs) != 1 {
		return fmt.Errorf("loaded %d packages for pattern %s, expected one", len(pkgs), pattern)
	}
	rt.thisPkgName = pkgs[0].Name
	rt.thisPkgPath = pkgs[0].PkgPath
	{
		resType, err := rt.resolveType(&cfg, pkgs[0], pi, pi.baseType)
		if err != nil {
			return fmt.Errorf("failed to resolve base type %s: %w", pi.baseType, err)
		}
		rt.resolvedBaseType = resType
	}
	for _, extType := range pi.extTypes {
		resType, err := rt.resolveType(&cfg, pkgs[0], pi, extType)
		if err != nil {
			return fmt.Errorf("failed to resolve ext type %s: %w", extType, err)
		}
		rt.resolvedExtTypes = append(rt.resolvedExtTypes, resType)
	}
	for _, ef := range pi.extraFields {
		efTypes, err := collectNamesFromAST(ef.expr)
		if err != nil {
			return fmt.Errorf("failed to collect type names from field type %s, likely an unsupported go type expression: %w", ef.typeStr, err)
		}
		for _, efType := range efTypes {
			pkg, realType, err := rt.resolveAnyType(&cfg, pkgs[0], pi, efType)
			if err != nil {
				return fmt.Errorf("failed to resolve a type %s from extra field type %s: %w", efType, ef.typeStr, err)
			}
			named, ok := realType.(*types.Named)
			if !ok {
				// all the efType are names in form of
				// either pkg.typename or typename, so
				// the realType can be either a named
				// type or a basic type. If it's a
				// basic type, then let's ignore it -
				// there is nothing to import for it
				// anyway.
				continue
			}
			resType := wrapIntoResolvedType(efType, pkg, named)
			rt.resolvedEfTypes = append(rt.resolvedEfTypes, resType)
		}
	}
	return nil
}

func collectNamesFromAST(a ast.Expr) ([]aType, error) {
	if a == nil {
		return nil, fmt.Errorf("nil ast node")
	}
	switch t := a.(type) {
	case *ast.Ident:
		return []aType{
			{
				pkgName: "",
				name:    t.Name,
			},
		}, nil
	case *ast.SelectorExpr:
		xident, ok := t.X.(*ast.Ident)
		if !ok || xident == nil || t.Sel == nil {
			return nil, fmt.Errorf("can't parse ast selector expression")
		}
		return []aType{
			{
				pkgName: xident.Name,
				name:    t.Sel.Name,
			},
		}, nil
	case *ast.ArrayType:
		return collectNamesFromAST(t.Elt)
	case *ast.StarExpr:
		return collectNamesFromAST(t.X)
	case *ast.FuncType:
		var types []aType
		for _, field := range t.Params.List {
			ptypes, err := collectNamesFromAST(field.Type)
			if err != nil {
				return nil, err
			}
			types = append(types, ptypes...)
		}
		if t.Results == nil {
			return types, nil
		}
		for _, field := range t.Results.List {
			rtypes, err := collectNamesFromAST(field.Type)
			if err != nil {
				return nil, err
			}
			types = append(types, rtypes...)
		}
		return types, nil
	case *ast.MapType:
		keyTypes, err := collectNamesFromAST(t.Key)
		if err != nil {
			return nil, err
		}
		valueTypes, err := collectNamesFromAST(t.Value)
		if err != nil {
			return nil, err
		}
		return append(keyTypes, valueTypes...), nil
	case *ast.ChanType:
		return collectNamesFromAST(t.Value)
	}
	return nil, nil
}

func (rt *resolvedTypes) resolveType(cfg *packages.Config, thisPkg *packages.Package, pi *parsedInput, typeToResolve aType) (resolvedType, error) {
	nilrt := resolvedType{}
	pkg, realType, err := rt.resolveAnyType(cfg, thisPkg, pi, typeToResolve)
	if err != nil {
		return nilrt, err
	}
	named, ok := realType.(*types.Named)
	if !ok {
		return nilrt, fmt.Errorf("type %s is not a named type", typeToResolve)
	}
	return wrapIntoResolvedType(typeToResolve, pkg, named), nil
}

func wrapIntoResolvedType(typeToResolve aType, pkg *packages.Package, named *types.Named) resolvedType {
	if pkg == nil {
		return resolvedType{
			at: typeToResolve,
			rt: named,
		}
	}
	return resolvedType{
		at:          typeToResolve,
		rt:          named,
		origPkgName: pkg.Name,
		pkgPath:     pkg.PkgPath,
	}
}

func (rt *resolvedTypes) resolveAnyType(cfg *packages.Config, thisPkg *packages.Package, pi *parsedInput, typeToResolve aType) (*packages.Package, types.Type, error) {
	pkgPath, err := getPkgPath(thisPkg, typeToResolve, pi.inFile, pi.imports)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get package path for type %s: %w (means, the package of the type is not imported in this package nor mentioned in -imports)", typeToResolve, err)
	}
	if pkgPath == "" {
		// no package name means one of the following:
		// - type comes from this package
		// - type is a builtin (error)
		// - type comes from a package imported with a dot
		//
		// last case is currently not supported
		realType, err := getType(thisPkg.Types.Scope(), typeToResolve.name)
		if err != nil {
			realType, err = getType(types.Universe, typeToResolve.name)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve the type %s in this package (%s) and in Universe: %w (means, we could not find the type in the actual package)", typeToResolve, thisPkg.PkgPath, err)
		}
		return nil, realType, nil
	}
	pkg, err := findPackage(cfg, thisPkg, pkgPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find package %s for type %s: %w (means, it isn't imported in this package, nor the go tools loader could load it", pkgPath, typeToResolve, err)
	}
	realType, err := getType(pkg.Types.Scope(), typeToResolve.name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve the type %s in pkg %s: %w (means, we could not find the type in the actual package)", typeToResolve, pkg.Name, err)
	}
	return pkg, realType, nil
}

type pkgPathAndName struct {
	pkgPath  string
	typeName string
}

func (i pkgPathAndName) String() string {
	if i.pkgPath != "" {
		return fmt.Sprintf(`"%s".%s`, i.pkgPath, i.typeName)
	}
	return i.typeName
}

type parameterInfo struct {
	name    string
	typeStr string
}

type methodInfo struct {
	name        string
	parameters  []parameterInfo
	returnTypes []string
}

type interfaceInfo struct {
	embeddedTypes   []pkgPathAndName
	explicitMethods []methodInfo
}

type processedType struct {
	info  pkgPathAndName
	iface *types.Interface
}

type typeAnalysis struct {
	thisPkgPath string
	imports     map[string]string                   // pkg path -> pkg name
	typeInfo    map[string]map[string]interfaceInfo // pkg path -> type name -> interface info
	typeQueue   []processedType
}

func (ta *typeAnalysis) analyze(rt *resolvedTypes, imports []anImport) error {
	ta.thisPkgPath = rt.thisPkgPath
	ta.imports = make(map[string]string)
	ta.typeInfo = make(map[string]map[string]interfaceInfo)
	importsMap := make(map[string]string, len(imports))
	for _, imprt := range imports {
		if _, ok := importsMap[imprt.path]; ok {
			return fmt.Errorf("duplicate entry in input imports for path %s", imprt.path)
		}
		importsMap[imprt.path] = imprt.name
	}
	if err := ta.analyzeForImports(rt, importsMap); err != nil {
		return err
	}
	if err := ta.analyzeForExtraImportsTypesAndMethods(rt); err != nil {
		return err
	}
	return nil
}

func (ta *typeAnalysis) analyzeForImports(rt *resolvedTypes, importsMap map[string]string) error {
	if err := ta.analyzeResolvedTypeForImports(rt.resolvedBaseType, importsMap); err != nil {
		return err
	}
	for _, resType := range rt.resolvedExtTypes {
		if err := ta.analyzeResolvedTypeForImports(resType, importsMap); err != nil {
			return err
		}
	}
	for _, resType := range rt.resolvedEfTypes {
		if err := ta.analyzeResolvedTypeForImports(resType, importsMap); err != nil {
			return err
		}
	}
	return nil
}

func (ta *typeAnalysis) analyzeResolvedTypeForImports(resType resolvedType, importsMap map[string]string) error {
	if resType.pkgPath == "" {
		return nil // builtin type, nothing to import
	}
	if resType.pkgPath == ta.thisPkgPath {
		// type from this package, nothing to import
		return nil
	}
	overriddenName, ok := ta.imports[resType.pkgPath]
	if ok {
		if overriddenName == "" {
			if resType.origPkgName != resType.at.pkgName {
				return fmt.Errorf("inconsistent imported package name, package %s is referred as %s and as %s, either fix the name in -imports or -basetype or -exttypes", resType.pkgPath, resType.origPkgName, resType.at.pkgName)
			}
		} else if overriddenName != resType.at.pkgName {
			return fmt.Errorf("inconsistent imported package name, package %s is referred as %s and as %s, either fix the name in -imports or -basetype or -exttypes", resType.pkgPath, overriddenName, resType.at.pkgName)
		}
	} else {
		if resType.origPkgName != resType.at.pkgName {
			overriddenName = resType.at.pkgName
			importName, ok := importsMap[resType.pkgPath]
			if ok {
				if importName != overriddenName {
					return fmt.Errorf("inconsistent imported package name, package %s is referred as %s and as %s, either fix the name in -imports or -basetype or -exttypes", resType.pkgPath, overriddenName, importName)
				}
			}
		} else {
			overriddenName = ""
			importName, ok := importsMap[resType.pkgPath]
			if ok {
				if importName != resType.origPkgName {
					return fmt.Errorf("inconsistent imported package name, package %s is referred as %s and as %s, either fix the name in -imports or -basetype or -exttypes", resType.pkgPath, resType.origPkgName, importName)
				}
			}
		}
		ta.imports[resType.pkgPath] = overriddenName
	}
	return nil
}

func (ta *typeAnalysis) analyzeForExtraImportsTypesAndMethods(rt *resolvedTypes) error {
	if err := ta.analyzeResolvedTypeForExtraImportsTypesAndMethods(rt.resolvedBaseType); err != nil {
		return err
	}
	for _, resType := range rt.resolvedExtTypes {
		if err := ta.analyzeResolvedTypeForExtraImportsTypesAndMethods(resType); err != nil {
			return err
		}
	}
	return nil
}

func (ta *typeAnalysis) analyzeResolvedTypeForExtraImportsTypesAndMethods(resType resolvedType) error {
	info := pkgPathAndName{
		pkgPath:  resType.pkgPath,
		typeName: resType.at.name,
	}
	if ta.contains(info) {
		return nil
	}
	underType := resType.rt.Underlying()
	underIface, ok := underType.(*types.Interface)
	if !ok {
		return fmt.Errorf("%s is not an interface", resType.at)
	}
	err := ta.analyzeInterface(info, underIface)
	if err != nil {
		return fmt.Errorf("failed to analyze resolved type for imports, types and methods %s: %w", resType.at, err)
	}
	return nil
}

func (ta *typeAnalysis) analyzeInterface(info pkgPathAndName, iface *types.Interface) error {
	ta.addForAnalysis(info, iface)
	for len(ta.typeQueue) > 0 {
		pt := ta.typeQueue[0]
		ta.typeQueue[0] = processedType{}
		ta.typeQueue = ta.typeQueue[1:]
		if ta.contains(pt.info) {
			continue
		}
		embeddedTypes, err := ta.analyzeEmbeddedTypes(iface)
		if err != nil {
			return err
		}
		explicitMethods, err := ta.analyzeExplicitMethods(iface)
		if err != nil {
			return err
		}
		ta.insert(pt.info, embeddedTypes, explicitMethods)
	}
	ta.typeQueue = nil
	return nil
}

func (ta *typeAnalysis) insert(info pkgPathAndName, embeddedTypes []pkgPathAndName, explicitMethods []methodInfo) {
	typeNameToInfos, ok := ta.typeInfo[info.pkgPath]
	if !ok {
		typeNameToInfos = make(map[string]interfaceInfo)
		ta.typeInfo[info.pkgPath] = typeNameToInfos
	}
	typeNameToInfos[info.typeName] = interfaceInfo{
		embeddedTypes:   embeddedTypes,
		explicitMethods: explicitMethods,
	}
}

func (ta *typeAnalysis) analyzeEmbeddedTypes(iface *types.Interface) ([]pkgPathAndName, error) {
	infos := make([]pkgPathAndName, 0, iface.NumEmbeddeds())
	for idx := 0; idx < iface.NumEmbeddeds(); idx++ {
		et := iface.EmbeddedType(idx)
		named, ok := et.(*types.Named)
		if !ok {
			return nil, fmt.Errorf("embedded type %s is not an named type (%#v)", et, et)
		}
		obj := named.Obj()
		eat := aType{
			pkgName: "",
			name:    obj.Name(),
		}
		pkgPath := ""
		if pkg := obj.Pkg(); pkg != nil {
			pkgPath = pkg.Path()
			if name, ok := ta.imports[pkgPath]; ok {
				if name != "" {
					eat.pkgName = name
				}
			} else {
				ta.imports[pkgPath] = ""
			}
			if eat.pkgName == "" {
				eat.pkgName = pkg.Name()
			}
		}
		info := pkgPathAndName{
			pkgPath:  pkgPath,
			typeName: eat.name,
		}
		infos = append(infos, info)
		if ta.contains(info) {
			continue
		}
		underType := named.Underlying()
		underIface, ok := underType.(*types.Interface)
		if !ok {
			return nil, fmt.Errorf("embedded type %s is not a named interface type (%#v)", eat, underType)
		}
		ta.addForAnalysis(info, underIface)
	}
	return infos, nil
}

func (ta *typeAnalysis) addForAnalysis(info pkgPathAndName, iface *types.Interface) {
	ta.typeQueue = append(ta.typeQueue, processedType{
		info:  info,
		iface: iface,
	})
}

func (ta *typeAnalysis) contains(info pkgPathAndName) bool {
	if typeNameToInfos, ok := ta.typeInfo[info.pkgPath]; ok {
		if _, ok := typeNameToInfos[info.typeName]; ok {
			return true
		}
	}
	return false
}

func (ta *typeAnalysis) get(info pkgPathAndName) (interfaceInfo, bool) {
	typeNameToInfos, ok := ta.typeInfo[info.pkgPath]
	if !ok {
		return interfaceInfo{}, false
	}
	ifaceInfo, ok := typeNameToInfos[info.typeName]
	return ifaceInfo, ok
}

func (ta *typeAnalysis) mustGet(info pkgPathAndName) interfaceInfo {
	ifaceInfo, ok := ta.get(info)
	if !ok {
		bug("no interface info for %s", info)
	}
	return ifaceInfo
}

func (ta *typeAnalysis) analyzeExplicitMethods(iface *types.Interface) ([]methodInfo, error) {
	infos := make([]methodInfo, 0, iface.NumExplicitMethods())
	for idx := 0; idx < iface.NumExplicitMethods(); idx++ {
		m := iface.ExplicitMethod(idx)
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			return nil, fmt.Errorf("function %s has no signature", m.Name())
		}
		params, err := ta.tupleToParameters(sig.Params())
		if err != nil {
			return nil, err
		}
		results, err := ta.tupleToTypes(sig.Results())
		if err != nil {
			return nil, err
		}
		infos = append(infos, methodInfo{
			name:        m.Name(),
			parameters:  params,
			returnTypes: results,
		})
	}
	return infos, nil
}

func (ta *typeAnalysis) tupleToTypes(tuple *types.Tuple) ([]string, error) {
	types := make([]string, 0, tuple.Len())
	for idx := 0; idx < tuple.Len(); idx++ {
		str, err := ta.typeToStr(tuple.At(idx).Type())
		if err != nil {
			return nil, err
		}
		types = append(types, str)
	}
	return types, nil
}

func (ta *typeAnalysis) typeToStr(vType types.Type) (string, error) {
	switch vRealType := vType.(type) {
	case *types.Basic:
		return vRealType.Name(), nil
	case *types.Pointer:
		elemStr, err := ta.typeToStr(vRealType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("*%s", elemStr), nil
	case *types.Array:
		elemStr, err := ta.typeToStr(vRealType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("[%d]%s", vRealType.Len(), elemStr), nil
	case *types.Slice:
		elemStr, err := ta.typeToStr(vRealType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("[]%s", elemStr), nil
	case *types.Map:
		keyStr, err := ta.typeToStr(vRealType.Key())
		if err != nil {
			return "", err
		}
		elemStr, err := ta.typeToStr(vRealType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map[%s]%s", keyStr, elemStr), nil
	case *types.Chan:
		elemStr, err := ta.typeToStr(vRealType.Elem())
		if err != nil {
			return "", nil
		}
		switch vRealType.Dir() {
		case types.SendRecv:
			if c, ok := vRealType.Elem().(*types.Chan); ok && c.Dir() == types.RecvOnly {
				return fmt.Sprintf("chan (%s)", elemStr), nil
			}
			return fmt.Sprintf("chan %s", elemStr), nil
		case types.RecvOnly:
			return fmt.Sprintf("<-chan %s", elemStr), nil
		case types.SendOnly:
			return fmt.Sprintf("chan<- %s", elemStr), nil
		}
		return "", fmt.Errorf("invalid channel direction %v", vRealType.Dir())
	case *types.Struct:
		return "", errors.New("bare struct types are not supported")
	case *types.Tuple:
		return "", errors.New("tuple types are not supported")
	case *types.Signature:
		params, err := ta.paramTupleToTypesString(vRealType.Params(), vRealType.Variadic())
		if err != nil {
			return "", err
		}
		retvals, err := ta.retvalTupleToTypesString(vRealType.Results())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func %s %s", params, retvals), nil
	case *types.Named:
		vNamedTypeObj := vRealType.Obj()
		vName := vNamedTypeObj.Name()
		vPkg := vNamedTypeObj.Pkg()
		if vPkg == nil {
			return vName, nil
		}
		vPkgPath := vPkg.Path()
		if vPkgPath == ta.thisPkgPath {
			return vName, nil
		}
		vPkgName := vPkg.Name()
		if name, ok := ta.imports[vPkgPath]; ok {
			if name != "" {
				vPkgName = name
			}
		} else {
			ta.imports[vPkgPath] = ""
		}
		return fmt.Sprintf("%s.%s", vPkgName, vName), nil
	case *types.Interface:
		// TODO: check if this gets triggered for interface{}
		// parameters
		return "", errors.New("bare interface types are not supported")
	}
	return "", fmt.Errorf("unknown type %#v", vType)
}

func (ta *typeAnalysis) paramTupleToTypesString(tuple *types.Tuple, variadic bool) (string, error) {
	types := make([]string, 0, tuple.Len())
	for idx := 0; idx < tuple.Len(); idx++ {
		str, err := ta.typeToStr(tuple.At(idx).Type())
		if err != nil {
			return "", err
		}
		types = append(types, str)
	}
	if variadic {
		types[tuple.Len()-1] = fmt.Sprintf("...%s", types[tuple.Len()-1])
	}
	joined := strings.Join(types, ", ")
	return fmt.Sprintf("(%s)", joined), nil
}

func (ta *typeAnalysis) retvalTupleToTypesString(tuple *types.Tuple) (string, error) {
	types, err := ta.tupleToTypes(tuple)
	if err != nil {
		return "", err
	}
	if len(types) == 1 {
		return types[0], nil
	}
	joined := strings.Join(types, ", ")
	return fmt.Sprintf("(%s)", joined), nil
}

func (ta *typeAnalysis) tupleToParameters(t *types.Tuple) ([]parameterInfo, error) {
	if t == nil || t.Len() == 0 {
		return nil, nil
	}
	var params []parameterInfo
	for idx := 0; idx < t.Len(); idx++ {
		v := t.At(idx)
		vName := v.Name()
		vType := v.Type()
		vTypeStr, err := ta.typeToStr(vType)
		if err != nil {
			return nil, fmt.Errorf("could not handle parameter %s: %w", vName, err)
		}
		params = append(params, parameterInfo{
			name:    vName,
			typeStr: vTypeStr,
		})
	}
	return params, nil
}

func printNewFunc(w io.Writer, funcName, prefix string, rt *resolvedTypes, extraFields []extraField) {
	varName := fmt.Sprintf("%s%s", prefix, rt.resolvedBaseType.at.name)
	en := rt.resolvedBaseType.at.StringNoDot()
	// exclude the zero - it will be handled after the switch
	fmt.Fprintf(w, "func %s(%s %s", funcName, varName, rt.resolvedBaseType.at)
	for _, ef := range extraFields {
		fmt.Fprintf(w, ", %s %s", ef.name, ef.typeStr)
	}
	fmt.Fprintf(w, ") %s {\n", rt.resolvedBaseType.at)
	nComb := NCombs(len(rt.resolvedExtTypes))
	if nComb > 1 {
		fmt.Fprintf(w, "\tswitch r := %s.(type) {\n", varName)
		for counter := nComb - 1; counter > 0; counter-- {
			tbn := fmt.Sprintf("%s%d", en, counter)
			fmt.Fprintf(w, "\tcase i%s:\n\t\treturn &t%s{\n\t\t\tr: r,\n", tbn, tbn)
			for _, ef := range extraFields {
				fmt.Fprintf(w, "\t\t\t%s: %s,\n", ef.name, ef.name)
			}
			fmt.Fprintf(w, "\t\t}\n")
		}
		fmt.Fprintf(w, "\t}\n")
	}
	fmt.Fprintf(w, "\treturn &t%s0{\n\t\tr: %s,\n", en, varName)
	for _, ef := range extraFields {
		fmt.Fprintf(w, "\t\t%s: %s,\n", ef.name, ef.name)
	}
	fmt.Fprintf(w, "\t}\n}\n")
}

type parametersFull []parameterInfo

func (p parametersFull) String() string {
	strs := make([]string, 0, len(p))
	names := StringSet{}
	for idx, e := range p {
		name := generateName(names, e.name, idx)
		strs = append(strs, fmt.Sprintf("%s %s", name, e.typeStr))
	}
	return strings.Join(strs, ", ")
}

type parametersNames []parameterInfo

func (p parametersNames) String() string {
	strs := make([]string, 0, len(p))
	names := StringSet{}
	for idx, e := range p {
		name := generateName(names, e.name, idx)
		strs = append(strs, name)
	}
	return strings.Join(strs, ", ")
}

func generateName(names StringSet, name string, idx int) string {
	if name == "" {
		name = fmt.Sprintf("param%d", idx)
	}
	for names.Has(name) {
		idx *= 10
		name = fmt.Sprintf("param%d", idx)
	}
	names.Add(name)
	return name
}

func printImpls(w io.Writer, rt *resolvedTypes, ta *typeAnalysis, prefix string, extraFields []extraField) {
	comb := NewCombGen(len(rt.resolvedExtTypes))
	counter := 0
	en := rt.resolvedBaseType.at.StringNoDot()
	first := true
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		if first {
			first = false
		} else {
			fmt.Fprintf(w, "\n")
		}
		handled := printImplsFromResolvedType(w, rt.resolvedBaseType, ta, tbn, prefix, extraFields, nil)
		for _, idx := range idxs {
			handled = printImplsFromResolvedType(w, rt.resolvedExtTypes[idx], ta, tbn, prefix, extraFields, handled)
		}
		counter++
	}
}

func printExplicitImplsOfInterface(w io.Writer, info pkgPathAndName, ta *typeAnalysis, tbn, prefix string, extraFields []extraField) {
	ifaceInfo := ta.mustGet(info)
	for _, mi := range ifaceInfo.explicitMethods {
		fmt.Fprintf(w, "func (o%s *t%s) %s(%s)", tbn, tbn, mi.name, (parametersFull)(mi.parameters))
		switch len(mi.returnTypes) {
		case 0:
			// nothing to print
		case 1:
			fmt.Fprintf(w, " %s", mi.returnTypes[0])
		default:
			fmt.Fprintf(w, " (%s)", strings.Join(mi.returnTypes, ", "))
		}
		fmt.Fprintf(w, " {\n\t")
		if len(mi.returnTypes) > 0 {
			fmt.Fprintf(w, "return ")
		}
		fmt.Fprintf(w, "%s%s(o%s.r", prefix, mi.name, tbn)
		for _, ef := range extraFields {
			fmt.Fprintf(w, ", o%s.%s", tbn, ef.name)
		}
		if len(mi.parameters) > 0 {
			fmt.Fprintf(w, ", %s", (parametersNames)(mi.parameters))
		}
		fmt.Fprintf(w, ")\n}\n")
	}
}

func printImplsOfEmbeddedTypes(w io.Writer, info pkgPathAndName, ta *typeAnalysis, excludes StringSet, tbn, prefix string, extraFields []extraField) StringSet {
	newExcludes := StringSet{}
	ifaceInfo := ta.mustGet(info)
	for _, eti := range ifaceInfo.embeddedTypes {
		etiStr := eti.String()
		if excludes.Has(etiStr) {
			continue
		}
		newExcludes.Add(etiStr)
		subExcludes := printImplsFromInterfaceRecursive(w, eti, ta, newExcludes, tbn, prefix, extraFields)
		newExcludes.AddSet(subExcludes)
	}
	return newExcludes
}

func printImplsFromInterfaceRecursive(w io.Writer, info pkgPathAndName, ta *typeAnalysis, excludes StringSet, tbn, prefix string, extraFields []extraField) StringSet {
	subExcludes := printImplsOfEmbeddedTypes(w, info, ta, excludes, tbn, prefix, extraFields)
	printExplicitImplsOfInterface(w, info, ta, tbn, prefix, extraFields)
	newExcludes := StringSet{}
	newExcludes.AddSet(excludes)
	newExcludes.AddSet(subExcludes)
	return newExcludes
}

func printImplsFromResolvedType(w io.Writer, resType resolvedType, ta *typeAnalysis, tbn, prefix string, extraFields []extraField, excludes StringSet) StringSet {
	info := pkgPathAndName{
		pkgPath:  resType.pkgPath,
		typeName: resType.at.name,
	}
	newExcludes := StringSet{}
	newExcludes.AddSet(excludes)
	newExcludes.Add(info.String())
	subExcludes := printImplsFromInterfaceRecursive(w, info, ta, newExcludes, tbn, prefix, extraFields)
	return subExcludes
}

func printVars(w io.Writer, rt *resolvedTypes) {
	fmt.Fprintf(w, "var (\n")
	counter := 0
	en := rt.resolvedBaseType.at.StringNoDot()
	comb := NewCombGen(len(rt.resolvedExtTypes))
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		fmt.Fprintf(w, "\t_ %s = &t%s{}\n", rt.resolvedBaseType.at, tbn)
		for _, idx := range idxs {
			fmt.Fprintf(w, "\t_ %s = &t%s{}\n", rt.resolvedExtTypes[idx].at, tbn)
		}
		counter++
	}
	fmt.Fprintf(w, ")\n")
}

func printTypes(w io.Writer, rt *resolvedTypes, extraFields []extraField) {
	fmt.Fprintf(w, "type (\n")
	counter := 0
	en := rt.resolvedBaseType.at.StringNoDot()
	comb := NewCombGen(len(rt.resolvedExtTypes))
	for comb.Next() {
		idxs := comb.Get()
		tbn := fmt.Sprintf("%s%d", en, counter)
		fmt.Fprintf(w, "\n\ti%s interface {\n\t\t%s\n", tbn, rt.resolvedBaseType.at)
		for _, idx := range idxs {
			fmt.Fprintf(w, "\t\t%s\n", rt.resolvedExtTypes[idx].at)
		}
		fmt.Fprintf(w, "\t}\n\n\tt%s struct {\n\t\tr i%s\n", tbn, tbn)
		for _, ef := range extraFields {
			fmt.Fprintf(w, "\t\t%s %s\n", ef.name, ef.typeStr)
		}
		fmt.Fprintf(w, "\t}\n")
		counter++
	}
	fmt.Fprintf(w, ")\n")
}

func printImports(w io.Writer, ta *typeAnalysis) {
	sortedImports := make([]string, 0, len(ta.imports))
	for pkgPath := range ta.imports {
		sortedImports = append(sortedImports, pkgPath)
	}
	sort.Strings(sortedImports)
	fmt.Fprintf(w, "import (\n")

	for _, pkgPath := range sortedImports {
		name, ok := ta.imports[pkgPath]
		if !ok {
			bug("corrupted imports, %#v and %#v", sortedImports, ta.imports)
		}
		if name != "" {
			fmt.Fprintf(w, "\t%s %q\n", name, pkgPath)
		} else {
			fmt.Fprintf(w, "\t%q\n", pkgPath)
		}
	}
	fmt.Fprintf(w, ")\n")
}

func getPkgPath(thisPkg *packages.Package, at aType, inFile string, imports []anImport) (string, error) {
	if at.pkgName == "" {
		return "", nil
	}
	for _, imprt := range imports {
		if imprt.name == at.pkgName {
			return imprt.path, nil
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

func getType(scope *types.Scope, name string) (types.Type, error) {
	obj := scope.Lookup(name)
	if obj != nil {
		return obj.Type(), nil
	}
	return nil, fmt.Errorf("no type %s", name)
}

func bug(formatStr string, args ...interface{}) {
	printWithPrefix("BUG", formatStr, args...)
	os.Exit(2)
}

func warn(formatStr string, args ...interface{}) {
	printWithPrefix("WARN", formatStr, args...)
}

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
