package typescriptify

import (
	"reflect"
)

type TypeConversionHandler interface {
	HandleTypeConversion(depth int, result string, t *TypeScriptify, builder *TypeScriptClassBuilder, typeOf reflect.Type, customCode map[string]string, field reflect.StructField, fldOpts TypeOptions, jsonFieldName string) (string, error)
}

type DefaultTypeConversionHandler struct {
}

func (handler *DefaultTypeConversionHandler) HandleTypeConversion(depth int, result string, t *TypeScriptify, builder *TypeScriptClassBuilder, typeOf reflect.Type, customCode map[string]string, field reflect.StructField, fldOpts TypeOptions, jsonFieldName string) (string, error) {
	var err error

	if fldOpts.TSDoc != "" {
		builder.AddFieldDefinitionLine("/** " + fldOpts.TSDoc + " */")
	}
	if fldOpts.TSTransform != "" {
		t.Logf(depth, "- simple field %s.%s", typeOf.Name(), field.Name)
		err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
	} else if t.IsEnum(field) {
		t.Logf(depth, "- enum field %s.%s", typeOf.Name(), field.Name)
		builder.AddEnumField(jsonFieldName, field)
	} else if fldOpts.TSType != "" { // Struct:
		t.Logf(depth, "- simple field 1 %s.%s", typeOf.Name(), field.Name)
		err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
	} else if field.Type.Kind() == reflect.Struct { // Struct:
		t.Logf(depth, "- struct %s.%s (%s)", typeOf.Name(), field.Name, field.Type.String())
		typeScriptChunk, err := t.ConvertType(depth+1, field.Type, customCode)
		if err != nil {
			return "", err
		}
		if typeScriptChunk != "" {
			result = typeScriptChunk + "\n" + result
		}
		builder.AddStructField(jsonFieldName, field)
	} else if field.Type.Kind() == reflect.Map {
		t.Logf(depth, "- map field %s.%s", typeOf.Name(), field.Name)
		// Also convert map key types if needed
		var keyTypeToConvert reflect.Type
		switch field.Type.Key().Kind() {
		case reflect.Struct:
			keyTypeToConvert = field.Type.Key()
		case reflect.Ptr:
			keyTypeToConvert = field.Type.Key().Elem()
		}
		if keyTypeToConvert != nil {
			typeScriptChunk, err := t.ConvertType(depth+1, keyTypeToConvert, customCode)
			if err != nil {
				return "", err
			}
			if typeScriptChunk != "" {
				result = typeScriptChunk + "\n" + result
			}
		}
		// Also convert map value types if needed
		var valueTypeToConvert reflect.Type
		switch field.Type.Elem().Kind() {
		case reflect.Struct:
			valueTypeToConvert = field.Type.Elem()
		case reflect.Ptr:
			valueTypeToConvert = field.Type.Elem().Elem()
		}
		if valueTypeToConvert != nil {
			typeScriptChunk, err := t.ConvertType(depth+1, valueTypeToConvert, customCode)
			if err != nil {
				return "", err
			}
			if typeScriptChunk != "" {
				result = typeScriptChunk + "\n" + result
			}
		}

		builder.AddMapField(jsonFieldName, field)
	} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array { // Slice:
		if field.Type.Elem().Kind() == reflect.Ptr { //extract ptr type
			field.Type = field.Type.Elem()
		}

		arrayDepth := 1
		for field.Type.Elem().Kind() == reflect.Slice { // Slice of slices:
			field.Type = field.Type.Elem()
			arrayDepth++
		}

		if field.Type.Elem().Kind() == reflect.Struct { // Slice of structs:
			t.Logf(depth, "- struct slice %s.%s (%s)", typeOf.Name(), field.Name, field.Type.String())
			typeScriptChunk, err := t.ConvertType(depth+1, field.Type.Elem(), customCode)
			if err != nil {
				return "", err
			}
			if typeScriptChunk != "" {
				result = typeScriptChunk + "\n" + result
			}
			builder.AddArrayOfStructsField(jsonFieldName, field, arrayDepth)
		} else { // Slice of simple fields:
			t.Logf(depth, "- slice field %s.%s", typeOf.Name(), field.Name)
			err = builder.AddSimpleArrayField(jsonFieldName, field, arrayDepth, fldOpts)
		}
	} else { // Simple field:
		t.Logf(depth, "- simple field 2 %s.%s", typeOf.Name(), field.Name)
		err = builder.AddSimpleField(jsonFieldName, field, fldOpts)
	}
	return result, err
}
