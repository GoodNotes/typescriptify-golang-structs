package typescriptify

import (
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/fatih/structtag"
	"github.com/tkrajina/go-reflector/reflector"
	"golang.org/x/exp/slices"
)

const (
	tsDocTag            = "ts_doc"
	tsTransformTag      = "ts_transform"
	tsType              = "ts_type"
	tsConvertValuesFunc = `convertValues(a: any, classs: any, asMap: boolean = false): any {
	if (!a) {
		return a;
	}
	if (a.slice) {
		return (a as any[]).map(elem => this.convertValues(elem, classs));
	} else if ("object" === typeof a) {
		if (asMap) {
			for (const key of Object.keys(a)) {
				a[key] = new classs(a[key]);
			}
			return a;
		}
		return new classs(a);
	}
	return a;
}`
)

// TypeOptions overrides options set by `ts_*` tags.
type TypeOptions struct {
	TSType      string
	TSDoc       string
	TSTransform string
}

// FieldTags add any tags to a field.
type FieldTags map[string][]*structtag.Tag

// Set tags to struct fields
func AddFieldTags(t reflect.Type, fieldTags *FieldTags) reflect.Type {
	sf := make([]reflect.StructField, 0)
	for i := 0; i < t.NumField(); i++ {
		sf = append(sf, t.Field(i))

		if newTags, ok := (*fieldTags)[t.Field(i).Name]; ok {
			// parse field Tag
			tagString := string(t.Field(i).Tag)
			tags, err := structtag.Parse(tagString)
			if err != nil {
				fmt.Printf("Error parsing %q: %v\n", tagString, err)
				continue
			}
			// set newTags
			for _, tag := range newTags {
				err := tags.Set(tag)
				if err != nil {
					fmt.Printf("Error setting tag %q: %v\n", tag, err)
				}
			}
			sf[i].Tag = reflect.StructTag(tags.String())
		}
	}
	return reflect.StructOf(sf)
}

// Create anonymous struct with provided new tags added to all fields
func TagAll(t reflect.Type, newTags []string) reflect.Type {
	sf := make([]reflect.StructField, 0)
	for i := 0; i < t.NumField(); i++ {
		sf = append(sf, t.Field(i))

		// parse field Tag
		tagString := string(t.Field(i).Tag)
		tags, err := structtag.Parse(tagString)
		if err != nil {
			fmt.Printf("Error parsing %q: %v\n", tagString, err)
			continue
		}
		// add newTags to json tag
		jsonTag, err := tags.Get("json")
		if err != nil {
			fmt.Printf("Error getting json tag: %s\n", err)
			continue
		}
		jsonTag.Options = newTags
		err = tags.Set(jsonTag)
		if err != nil {
			fmt.Printf("Error setting %q tags: %s\n", newTags, err)
		}
		sf[i].Tag = reflect.StructTag(tags.String())
	}
	return reflect.StructOf(sf)
}

// StructType stores settings for transforming one Golang struct.
type StructType struct {
	Type         reflect.Type
	FieldOptions map[reflect.Type]TypeOptions
	TypeHandlers map[reflect.Type]TypeConversionHandler
	Name         string
}

func NewStruct(i interface{}) *StructType {
	return &StructType{
		Type: reflect.TypeOf(i),
	}
}

func (st *StructType) WithFieldOpts(i interface{}, opts TypeOptions) *StructType {
	if st.FieldOptions == nil {
		st.FieldOptions = map[reflect.Type]TypeOptions{}
	}
	var typ reflect.Type
	if ty, is := i.(reflect.Type); is {
		typ = ty
	} else {
		typ = reflect.TypeOf(i)
	}
	st.FieldOptions[typ] = opts
	return st
}

func (st *StructType) WithTypeHandler(i interface{}, handler TypeConversionHandler) *StructType {
	if st.TypeHandlers == nil {
		st.TypeHandlers = map[reflect.Type]TypeConversionHandler{}
	}
	var typ reflect.Type
	if ty, is := i.(reflect.Type); is {
		typ = ty
	} else {
		typ = reflect.TypeOf(i)
	}
	st.TypeHandlers[typ] = handler
	return st
}

type EnumType struct {
	Type reflect.Type
}

type EnumElement struct {
	Value interface{}
	Name  string
}

type TypeScriptify struct {
	Prefix string
	Suffix string
	Indent string
	// deprecated: use CreateConstructor
	CreateFromMethod  bool
	CreateConstructor bool
	BackupDir         string // If empty no backup
	DontExport        bool
	CreateInterface   bool
	ReadOnlyFields    bool
	CamelCaseFields   bool
	CamelCaseOptions  *CamelCaseOptions
	customImports     []string

	structTypes                  []StructType
	enumTypes                    []EnumType
	enums                        map[reflect.Type][]EnumElement
	kinds                        map[reflect.Kind]string
	DefaultTypeConversionHandler TypeConversionHandler

	fieldTypeOptions map[reflect.Type]TypeOptions
	typeHandlers     map[reflect.Type]TypeConversionHandler

	// throwaway, used when converting
	alreadyConverted map[reflect.Type]bool
}

func New() *TypeScriptify {
	result := new(TypeScriptify)
	result.Indent = "\t"
	result.BackupDir = "."

	kinds := make(map[reflect.Kind]string)

	kinds[reflect.Bool] = "boolean"
	kinds[reflect.Interface] = "any"

	kinds[reflect.Int] = "number"
	kinds[reflect.Int8] = "number"
	kinds[reflect.Int16] = "number"
	kinds[reflect.Int32] = "number"
	kinds[reflect.Int64] = "number"
	kinds[reflect.Uint] = "number"
	kinds[reflect.Uint8] = "number"
	kinds[reflect.Uint16] = "number"
	kinds[reflect.Uint32] = "number"
	kinds[reflect.Uint64] = "number"
	kinds[reflect.Float32] = "number"
	kinds[reflect.Float64] = "number"

	kinds[reflect.String] = "string"

	result.kinds = kinds
	result.DefaultTypeConversionHandler = &DefaultTypeConversionHandler{}

	result.Indent = "    "
	result.CreateFromMethod = false
	result.CreateConstructor = true

	return result
}

// DeepFields returns all fields of a struct, including fields of embedded structs.
func DeepFields(typeOf reflect.Type) []reflect.StructField {
	fields := make([]reflect.StructField, 0)

	if typeOf.Kind() == reflect.Ptr {
		typeOf = typeOf.Elem()
	}

	if typeOf.Kind() != reflect.Struct {
		return fields
	}

	for i := 0; i < typeOf.NumField(); i++ {
		f := typeOf.Field(i)

		kind := f.Type.Kind()
		if f.Anonymous && kind == reflect.Struct {
			//fmt.Println(v.Interface())
			fields = append(fields, DeepFields(f.Type)...)
		} else if f.Anonymous && kind == reflect.Ptr && f.Type.Elem().Kind() == reflect.Struct {
			//fmt.Println(v.Interface())
			fields = append(fields, DeepFields(f.Type.Elem())...)
		} else {
			fields = append(fields, f)
		}
	}

	return fields
}

func (ts TypeScriptify) Logf(depth int, s string, args ...interface{}) {
	fmt.Printf(strings.Repeat("   ", depth)+s+"\n", args...)
}

// ManageType can define custom options for fields of a specified type.
//
// This can be used instead of setting ts_type and ts_transform for all fields of a certain type.
func (t *TypeScriptify) ManageType(fld interface{}, opts TypeOptions) *TypeScriptify {
	var typ reflect.Type
	switch t := fld.(type) {
	case reflect.Type:
		typ = t
	default:
		typ = reflect.TypeOf(fld)
	}
	if t.fieldTypeOptions == nil {
		t.fieldTypeOptions = map[reflect.Type]TypeOptions{}
	}
	t.fieldTypeOptions[typ] = opts
	return t
}

// ManageTypeConversion can define custom conversion Handler for fields of one or more types.
//
// This can be used to register different conversion handlers per struct type.
func (t *TypeScriptify) ManageTypeConversion(handler TypeConversionHandler, flds ...interface{}) *TypeScriptify {
	for _, fld := range flds {
		var typ reflect.Type
		switch t := fld.(type) {
		case reflect.Type:
			typ = t
		default:
			typ = reflect.TypeOf(fld)
		}
		if t.typeHandlers == nil {
			t.typeHandlers = map[reflect.Type]TypeConversionHandler{}
		}
		t.typeHandlers[typ] = handler
	}
	return t
}

func (t *TypeScriptify) WithTypeConversionHandler(handler TypeConversionHandler) *TypeScriptify {
	t.DefaultTypeConversionHandler = handler
	return t
}

// deprecated: use WithConstructor
func (t *TypeScriptify) WithCreateFromMethod(b bool) *TypeScriptify {
	t.CreateFromMethod = b
	return t
}

func (t *TypeScriptify) WithInterface(b bool) *TypeScriptify {
	t.CreateInterface = b
	return t
}

func (t *TypeScriptify) WithReadonlyFields(b bool) *TypeScriptify {
	t.ReadOnlyFields = b
	return t
}

func (t *TypeScriptify) WithCamelCaseFields(b bool, opts *CamelCaseOptions) *TypeScriptify {
	t.CamelCaseFields = b
	t.CamelCaseOptions = opts
	return t
}

func (t *TypeScriptify) WithConstructor(b bool) *TypeScriptify {
	t.CreateConstructor = b
	return t
}

func (t *TypeScriptify) WithIndent(i string) *TypeScriptify {
	t.Indent = i
	return t
}

func (t *TypeScriptify) WithBackupDir(b string) *TypeScriptify {
	t.BackupDir = b
	return t
}

func (t *TypeScriptify) WithPrefix(p string) *TypeScriptify {
	t.Prefix = p
	return t
}

func (t *TypeScriptify) WithSuffix(s string) *TypeScriptify {
	t.Suffix = s
	return t
}

func (t *TypeScriptify) Add(obj interface{}) *TypeScriptify {
	switch ty := obj.(type) {
	case StructType:
		t.structTypes = append(t.structTypes, ty)
	case *StructType:
		t.structTypes = append(t.structTypes, *ty)
	case reflect.Type:
		t.AddType(ty)
	default:
		t.AddType(reflect.TypeOf(obj))
	}
	return t
}

func (t *TypeScriptify) AddType(typeOf reflect.Type) *TypeScriptify {
	t.structTypes = append(t.structTypes, StructType{Type: typeOf})
	return t
}

func (t *TypeScriptify) AddTypeWithName(typeOf reflect.Type, name string) *TypeScriptify {
	t.structTypes = append(t.structTypes, StructType{Type: typeOf, Name: name})
	return t
}

func (t *TypeScriptClassBuilder) AddMapField(fieldName string, field reflect.StructField) {
	keyType := field.Type.Key()
	valueType := field.Type.Elem()
	valueTypeName := valueType.Name()
	if name, ok := t.types[valueType.Kind()]; ok {
		valueTypeName = name
	}
	if valueType.Kind() == reflect.Array || valueType.Kind() == reflect.Slice {
		valueTypeName = valueType.Elem().Name() + "[]"
	}
	if valueType.Kind() == reflect.Ptr {
		valueTypeName = valueType.Elem().Name()
	}
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")

	keyTypeStr := keyType.Name()
	if name, ok := t.types[keyType.Kind()]; ok {
		keyTypeStr = name
	}
	// Key should always be string, no need for this:
	// _, isSimple := t.types[keyType.Kind()]
	// if !isSimple {
	// 	keyTypeStr = t.prefix + keyType.Name() + t.suffix
	// }

	if valueType.Kind() == reflect.Struct {
		if len(valueTypeName) > 0 {
			t.fields = append(t.fields, fmt.Sprintf("%s%s: {[key: %s]: %s};", t.indent, fieldName, keyTypeStr, t.prefix+valueTypeName+t.suffix))
			t.constructorBody = append(t.constructorBody, fmt.Sprintf("%s%sthis.%s = this.convertValues(source[\"%s\"], %s, true);", t.indent, t.indent, strippedFieldName, strippedFieldName, t.prefix+valueTypeName+t.suffix))
		} else {
			t.fields = append(t.fields, fmt.Sprintf("%s%s: {[key: %s]: any};", t.indent, fieldName, keyTypeStr))
			t.constructorBody = append(t.constructorBody, fmt.Sprintf("%s%sthis.%s = source[\"%s\"];", t.indent, t.indent, strippedFieldName, strippedFieldName))
		}
	} else {
		t.fields = append(t.fields, fmt.Sprintf("%s%s: {[key: %s]: %s};", t.indent, fieldName, keyTypeStr, valueTypeName))
		t.constructorBody = append(t.constructorBody, fmt.Sprintf("%s%sthis.%s = source[\"%s\"];", t.indent, t.indent, strippedFieldName, strippedFieldName))
	}
}

func (t *TypeScriptify) AddEnum(values interface{}) *TypeScriptify {
	if t.enums == nil {
		t.enums = map[reflect.Type][]EnumElement{}
	}
	items := reflect.ValueOf(values)
	if items.Kind() != reflect.Slice {
		panic(fmt.Sprintf("Values for %T isn't a slice", values))
	}

	var elements []EnumElement
	for i := 0; i < items.Len(); i++ {
		item := items.Index(i)

		var el EnumElement
		if item.Kind() == reflect.Struct {
			r := reflector.New(item.Interface())
			val, err := r.Field("Value").Get()
			if err != nil {
				panic(fmt.Sprint("missing Type field in ", item.Type().String()))
			}
			name, err := r.Field("TSName").Get()
			if err != nil {
				panic(fmt.Sprint("missing TSName field in ", item.Type().String()))
			}
			el.Value = val
			el.Name = name.(string)
		} else {
			el.Value = item.Interface()
			if tsNamer, is := item.Interface().(TSNamer); is {
				el.Name = tsNamer.TSName()
			} else {
				panic(fmt.Sprint(item.Type().String(), " has no TSName method"))
			}
		}

		elements = append(elements, el)
	}
	ty := reflect.TypeOf(elements[0].Value)
	t.enums[ty] = elements
	t.enumTypes = append(t.enumTypes, EnumType{Type: ty})

	return t
}

// AddEnumValues is deprecated, use `AddEnum()`
func (t *TypeScriptify) AddEnumValues(typeOf reflect.Type, values interface{}) *TypeScriptify {
	t.AddEnum(values)
	return t
}

func (t *TypeScriptify) Convert(customCode map[string]string) (string, error) {
	if t.CreateFromMethod {
		fmt.Fprintln(os.Stderr, "FromMethod METHOD IS DEPRECATED AND WILL BE REMOVED!!!!!!")
	}

	t.alreadyConverted = make(map[reflect.Type]bool)
	depth := 0

	result := ""
	if len(t.customImports) > 0 {
		// Put the custom imports, i.e.: `import Decimal from 'decimal.js'`
		for _, cimport := range t.customImports {
			result += cimport + "\n"
		}
	}

	for _, enumTyp := range t.enumTypes {
		elements := t.enums[enumTyp.Type]
		typeScriptCode, err := t.convertEnum(depth, enumTyp.Type, elements)
		if err != nil {
			return "", err
		}
		result += "\n" + strings.Trim(typeScriptCode, " "+t.Indent+"\r\n")
	}

	for _, strctTyp := range t.structTypes {
		typeScriptCode, err := t.ConvertType(depth, strctTyp.Type, customCode)
		if err != nil {
			return "", err
		}
		result += "\n" + strings.Trim(typeScriptCode, " "+t.Indent+"\r\n")
	}
	return result, nil
}

func loadCustomCode(fileName string) (map[string]string, error) {
	result := make(map[string]string)
	f, err := os.Open(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, err
	}
	defer f.Close()

	bytes, err := io.ReadAll(f)
	if err != nil {
		return result, err
	}

	var currentName string
	var currentValue string
	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "//[") && strings.HasSuffix(trimmedLine, ":]") {
			currentName = strings.Replace(strings.Replace(trimmedLine, "//[", "", -1), ":]", "", -1)
			currentValue = ""
		} else if trimmedLine == "//[end]" {
			result[currentName] = strings.TrimRight(currentValue, " \t\r\n")
			currentName = ""
			currentValue = ""
		} else if len(currentName) > 0 {
			currentValue += line + "\n"
		}
	}

	return result, nil
}

func (t TypeScriptify) backup(fileName string) error {
	fileIn, err := os.Open(fileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// No neet to backup, just return:
		return nil
	}
	defer fileIn.Close()

	bytes, err := io.ReadAll(fileIn)
	if err != nil {
		return err
	}

	_, backupFn := path.Split(fmt.Sprintf("%s-%s.backup", fileName, time.Now().Format("2006-01-02T15_04_05.99")))
	if t.BackupDir != "" {
		backupFn = path.Join(t.BackupDir, backupFn)
	}

	return os.WriteFile(backupFn, bytes, os.FileMode(0700))
}

func (t TypeScriptify) ConvertToFile(fileName string) error {
	if len(t.BackupDir) > 0 {
		err := t.backup(fileName)
		if err != nil {
			return err
		}
	}

	customCode, err := loadCustomCode(fileName)
	if err != nil {
		return err
	}

	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer f.Close()

	converted, err := t.Convert(customCode)
	if err != nil {
		return err
	}

	if _, err := f.WriteString("/* Do not change, this code is generated from Golang structs */\n\n"); err != nil {
		return err
	}
	if _, err := f.WriteString(converted); err != nil {
		return err
	}
	if err != nil {
		return err
	}

	return nil
}

type TSNamer interface {
	TSName() string
}

func (t *TypeScriptify) convertEnum(depth int, typeOf reflect.Type, elements []EnumElement) (string, error) {
	t.Logf(depth, "Converting enum %s", typeOf.String())
	if t.IsMarkedConverted(typeOf) {
		return "", nil
	}
	t.MarkConverted(typeOf)

	entityName := t.Prefix + typeOf.Name() + t.Suffix
	result := "enum " + entityName + " {\n"

	for _, val := range elements {
		result += fmt.Sprintf("%s%s = %#v,\n", t.Indent, val.Name, val.Value)
	}

	result += "}"

	if !t.DontExport {
		result = "export " + result
	}

	return result, nil
}

func (t *TypeScriptify) MarkConverted(typeOf reflect.Type) {
	t.alreadyConverted[typeOf] = true
}

func (t *TypeScriptify) IsMarkedConverted(typeOf reflect.Type) bool {
	if _, found := t.alreadyConverted[typeOf]; found {
		return true
	}
	return false
}

func (t *TypeScriptify) getFieldOptions(structType reflect.Type, field reflect.StructField) TypeOptions {
	// By default use options defined by tags:
	opts := TypeOptions{
		TSTransform: field.Tag.Get(tsTransformTag),
		TSType:      field.Tag.Get(tsType),
		TSDoc:       field.Tag.Get(tsDocTag),
	}

	overrides := []TypeOptions{}

	// But there is maybe an struct-specific override:
	for _, strct := range t.structTypes {
		if strct.FieldOptions == nil {
			continue
		}
		if strct.Type == structType {
			if fldOpts, found := strct.FieldOptions[field.Type]; found {
				overrides = append(overrides, fldOpts)
			}
		}
	}

	if fldOpts, found := t.fieldTypeOptions[field.Type]; found {
		overrides = append(overrides, fldOpts)
	}

	for _, o := range overrides {
		if o.TSTransform != "" {
			opts.TSTransform = o.TSTransform
		}
		if o.TSType != "" {
			opts.TSType = o.TSType
		}
	}

	return opts
}

func (t *TypeScriptify) getTypeConversionHandler(structType reflect.Type, field reflect.StructField) TypeConversionHandler {
	// find structType specific handler
	for _, strct := range t.structTypes {
		if strct.TypeHandlers == nil {
			continue
		}
		if strct.Type == structType {
			if handler, found := strct.TypeHandlers[field.Type]; found {
				return handler
			}
		}
	}

	// find type specific global handler
	if handler, found := t.typeHandlers[structType]; found {
		return handler
	}

	// return default handler
	return t.DefaultTypeConversionHandler
}

func (t *TypeScriptify) getJSONFieldName(field reflect.StructField, isPtr bool) string {
	jsonFieldName := ""
	jsonTag := field.Tag.Get("json")
	if len(jsonTag) > 0 {
		jsonTagParts := strings.Split(jsonTag, ",")
		if len(jsonTagParts) > 0 {
			jsonFieldName = strings.Trim(jsonTagParts[0], t.Indent)
			//`json:",omitempty"` is valid
			if jsonFieldName == "" {
				jsonFieldName = field.Name
			}
		}
		hasOmitEmpty := false
		ignored := false
		for _, t := range jsonTagParts {
			if t == "" {
				continue
			}
			if t == "omitempty" {
				hasOmitEmpty = true
				break
			}
			if t == "-" {
				ignored = true
				break
			}
		}
		if !ignored && isPtr || hasOmitEmpty {
			jsonFieldName = fmt.Sprintf("%s?", jsonFieldName)
		}
	} else if /*field.IsExported()*/ field.PkgPath == "" {
		jsonFieldName = field.Name
	}
	if t.CamelCaseFields {
		jsonFieldName = CamelCase(jsonFieldName, t.CamelCaseOptions)
	}
	return jsonFieldName
}

func (t *TypeScriptify) ConvertType(depth int, typeOf reflect.Type, customCode map[string]string) (string, error) {
	if t.IsMarkedConverted(typeOf) {
		return "", nil
	}
	t.Logf(depth, "Converting type %s", typeOf.String())

	t.MarkConverted(typeOf)

	typeName := typeOf.Name()
	if typeName == "" {
		idx := slices.IndexFunc(t.structTypes,
			func(structType StructType) bool {
				return typeOf == structType.Type
			})
		if idx >= 0 && t.structTypes[idx].Name != "" {
			typeName = t.structTypes[idx].Name
		} else {
			t.Logf(depth, "Use .AddTypeWithName to avoid any for %q", typeOf.Name())
			typeName = "any"
		}
	}
	if typeName == "any" {
		return "", nil
	}
	entityName := t.Prefix + typeName + t.Suffix
	result := ""
	if t.CreateInterface {
		result += fmt.Sprintf("interface %s {\n", entityName)
	} else {
		result += fmt.Sprintf("class %s {\n", entityName)
	}
	if !t.DontExport {
		result = "export " + result
	}
	builder := TypeScriptClassBuilder{
		types:          t.kinds,
		indent:         t.Indent,
		prefix:         t.Prefix,
		suffix:         t.Suffix,
		readOnlyFields: t.ReadOnlyFields,
	}

	fields := DeepFields(typeOf)
	for _, field := range fields {
		isPtr := field.Type.Kind() == reflect.Ptr
		if isPtr {
			field.Type = field.Type.Elem()
		}
		jsonFieldName := t.getJSONFieldName(field, isPtr)
		if len(jsonFieldName) == 0 || jsonFieldName == "-" {
			continue
		}

		var err error
		fldOpts := t.getFieldOptions(typeOf, field)
		typeConversionHandler := t.getTypeConversionHandler(typeOf, field)
		result, err = typeConversionHandler.HandleTypeConversion(depth, result, t, &builder, typeOf, customCode, field, fldOpts, jsonFieldName)

		if err != nil {
			return "", err
		}
	}

	if t.CreateFromMethod {
		t.CreateConstructor = true
	}

	result += strings.Join(builder.fields, "\n") + "\n"
	if !t.CreateInterface {
		constructorBody := strings.Join(builder.constructorBody, "\n")
		needsConvertValue := strings.Contains(constructorBody, "this.convertValues")
		if t.CreateFromMethod {
			result += fmt.Sprintf("\n%sstatic createFrom(source: any = {}) {\n", t.Indent)
			result += fmt.Sprintf("%s%sreturn new %s(source);\n", t.Indent, t.Indent, entityName)
			result += fmt.Sprintf("%s}\n", t.Indent)
		}
		if t.CreateConstructor {
			result += fmt.Sprintf("\n%sconstructor(source: any = {}) {\n", t.Indent)
			result += t.Indent + t.Indent + "if ('string' === typeof source) source = JSON.parse(source);\n"
			result += constructorBody + "\n"
			result += fmt.Sprintf("%s}\n", t.Indent)
		}
		if needsConvertValue && (t.CreateConstructor || t.CreateFromMethod) {
			result += "\n" + indentLines(strings.ReplaceAll(tsConvertValuesFunc, "\t", t.Indent), 1) + "\n"
		}
	}

	if customCode != nil {
		code := customCode[entityName]
		if len(code) != 0 {
			result += t.Indent + "//[" + entityName + ":]\n" + code + "\n\n" + t.Indent + "//[end]\n"
		}
	}

	result += "}"

	return result, nil
}

func (t *TypeScriptify) IsEnum(field reflect.StructField) bool {
	if _, isEnum := t.enums[field.Type]; isEnum {
		return true
	}
	return false
}

func (t *TypeScriptify) AddImport(i string) {
	for _, cimport := range t.customImports {
		if cimport == i {
			return
		}
	}

	t.customImports = append(t.customImports, i)
}

type TypeScriptClassBuilder struct {
	types                map[reflect.Kind]string
	indent               string
	fields               []string
	createFromMethodBody []string
	constructorBody      []string
	prefix, suffix       string
	readOnlyFields       bool
}

func (t *TypeScriptClassBuilder) AddSimpleArrayField(fieldName string, field reflect.StructField, arrayDepth int, opts TypeOptions) error {
	fieldType, kind := field.Type.Elem().Name(), field.Type.Elem().Kind()
	typeScriptType := t.types[kind]

	if len(fieldName) > 0 {
		strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
		if len(opts.TSType) > 0 {
			t.addField(fieldName, opts.TSType)
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
			return nil
		} else if len(typeScriptType) > 0 {
			t.addField(fieldName, fmt.Sprint(typeScriptType, strings.Repeat("[]", arrayDepth)))
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
			return nil
		}
	}

	return fmt.Errorf("cannot find type for %s (%s/%s)", kind.String(), fieldName, fieldType)
}

func (t *TypeScriptClassBuilder) AddSimpleField(fieldName string, field reflect.StructField, opts TypeOptions) error {
	fieldType, kind := field.Type.Name(), field.Type.Kind()

	typeScriptType := t.types[kind]
	if len(opts.TSType) > 0 {
		typeScriptType = opts.TSType
	}

	if len(typeScriptType) > 0 && len(fieldName) > 0 {
		strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
		t.addField(fieldName, typeScriptType)
		if opts.TSTransform == "" {
			t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
		} else {
			val := fmt.Sprintf(`source["%s"]`, strippedFieldName)
			expression := strings.Replace(opts.TSTransform, "__VALUE__", val, -1)
			t.addInitializerFieldLine(strippedFieldName, expression)
		}
		return nil
	}

	return fmt.Errorf("cannot find type for %s (%s/%s)", kind.String(), fieldName, fieldType)
}

func (t *TypeScriptClassBuilder) AddEnumField(fieldName string, field reflect.StructField) {
	fieldType := field.Type.Name()
	t.addField(fieldName, t.prefix+fieldType+t.suffix)
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("source[\"%s\"]", strippedFieldName))
}

func (t *TypeScriptClassBuilder) AddStructField(fieldName string, field reflect.StructField) {
	fieldType := field.Type.Name()
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addField(fieldName, t.prefix+fieldType+t.suffix)
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("this.convertValues(source[\"%s\"], %s)", strippedFieldName, t.prefix+fieldType+t.suffix))
}

func (t *TypeScriptClassBuilder) AddArrayOfStructsField(fieldName string, field reflect.StructField, arrayDepth int) {
	fieldType := field.Type.Elem().Name()
	strippedFieldName := strings.ReplaceAll(fieldName, "?", "")
	t.addField(fieldName, fmt.Sprint(t.prefix+fieldType+t.suffix, strings.Repeat("[]", arrayDepth)))
	t.addInitializerFieldLine(strippedFieldName, fmt.Sprintf("this.convertValues(source[\"%s\"], %s)", strippedFieldName, t.prefix+fieldType+t.suffix))
}

func (t *TypeScriptClassBuilder) addInitializerFieldLine(fld, initializer string) {
	t.createFromMethodBody = append(t.createFromMethodBody, fmt.Sprint(t.indent, t.indent, "result.", fld, " = ", initializer, ";"))
	t.constructorBody = append(t.constructorBody, fmt.Sprint(t.indent, t.indent, "this.", fld, " = ", initializer, ";"))
}

func (t *TypeScriptClassBuilder) AddFieldDefinitionLine(line string) {
	t.fields = append(t.fields, t.indent+line)
}

func (t *TypeScriptClassBuilder) addField(fld, fldType string) {
	ro := ""
	if t.readOnlyFields {
		ro = "readonly "
	}
	t.fields = append(t.fields, fmt.Sprint(t.indent, ro, fld, ": ", fldType, ";"))
}
