package typescriptify

import (
	"reflect"
	"testing"
	"time"
)

type customTypeConversionHandler struct {
}

func (handler *customTypeConversionHandler) HandleTypeConversion(depth int, result string, t *TypeScriptify, builder *TypeScriptClassBuilder, typeOf reflect.Type, customCode map[string]string, field reflect.StructField, fldOpts TypeOptions, jsonFieldName string) (string, error) {
	var err error
	t.Logf(depth, "CUSTOM HANDLER - %q - %q", typeOf.Name(), field.Name)
	timeFieldOptions := TypeOptions{TSType: "Date", TSTransform: "new Date(__VALUE__)"}
	if typeOf.Name() == "ConfigFile" {
		switch field.Name {
		case "OSVersion":
			t.Logf(depth, "- (custom) simple field %s.%s", typeOf.Name(), field.Name)
			err = builder.AddSimpleField("osVersion?", field, fldOpts)
			return result, err
		case "OSFeatures":
			t.Logf(depth, "- (custom) simple field %s.%s", typeOf.Name(), field.Name)
			err = builder.AddSimpleArrayField("osFeatures?", field, 1, fldOpts)
			return result, err
		case "Created":
			t.Logf(depth, "- (custom) simple field %s.%s", typeOf.Name(), field.Name)
			err = builder.AddSimpleField(jsonFieldName, field, timeFieldOptions)
			return result, err
		default:
			t.Logf(depth, "- calling default conversion for %s.%s", typeOf.Name(), field.Name)
			return t.DefaultTypeConversionHandler.HandleTypeConversion(depth, result, t, builder, typeOf, customCode, field, fldOpts, jsonFieldName)
		}
	}
	if typeOf.Name() == "History" && field.Type.Name() == "Time" {
		t.Logf(depth, "- (custom) simple field %s.%s (%s)", typeOf.Name(), field.Name, field.Type.Name())
		err = builder.AddSimpleField(jsonFieldName, field, timeFieldOptions)
		return result, err
	}
	return t.DefaultTypeConversionHandler.HandleTypeConversion(depth, result, t, builder, typeOf, customCode, field, fldOpts, jsonFieldName)
}

func TestCustomTypeConversionHandler(t *testing.T) {
	t.Parallel()
	// Time is a wrapper around time.Time to help with deep copying
	type Time struct {
		time.Time
	}
	type History struct {
		Author     string `json:"author,omitempty"`
		Created    Time   `json:"created,omitempty"`
		CreatedBy  string `json:"created_by,omitempty"`
		Comment    string `json:"comment,omitempty"`
		EmptyLayer bool   `json:"empty_layer,omitempty"`
	}
	type ConfigFile struct {
		Architecture  string    `json:"architecture"`
		Author        string    `json:"author,omitempty"`
		Container     string    `json:"container,omitempty"`
		Created       Time      `json:"created,omitempty"`
		DockerVersion string    `json:"docker_version,omitempty"`
		History       []History `json:"history,omitempty"`
		OS            string    `json:"os"`
		OSVersion     string    `json:"os.version,omitempty"`
		Variant       string    `json:"variant,omitempty"`
		OSFeatures    []string  `json:"os.features,omitempty"`
	}
	type Metadata struct {
		Size int64 `json:",omitempty"`

		// Container image
		ImageID     string     `json:",omitempty"`
		DiffIDs     []string   `json:",omitempty"`
		RepoTags    []string   `json:",omitempty"`
		RepoDigests []string   `json:",omitempty"`
		ImageConfig ConfigFile `json:",omitempty"`
	}

	converter := New().WithCamelCaseFields(true, nil)

	converter.ManageTypeConversion(&customTypeConversionHandler{}, reflect.TypeOf(ConfigFile{}))
	converter.Add(
		NewStruct(History{}).WithTypeHandler(reflect.TypeOf(Time{}), &customTypeConversionHandler{}),
	)
	converter.AddType(reflect.TypeOf(Metadata{}))
	converter.BackupDir = ""
	converter.ReadOnlyFields = true
	converter.CreateInterface = true

	desiredResult := `export interface History {
	readonly author?: string;
	readonly created?: Date;
	readonly created_by?: string;
	readonly comment?: string;
	readonly empty_layer?: boolean;
}
export interface ConfigFile {
	readonly architecture: string;
	readonly author?: string;
	readonly container?: string;
	readonly created?: Date;
	readonly docker_version?: string;
	readonly history?: History[];
	readonly os: string;
	readonly osVersion?: string;
	readonly variant?: string;
	readonly osFeatures?: string[];
}
export interface Metadata {
	readonly size?: number;
	readonly imageID?: string;
	readonly diffIDs?: string[];
	readonly repoTags?: string[];
	readonly repoDigests?: string[];
	readonly imageConfig?: ConfigFile;
}`
	testConverter(t, converter, false, desiredResult, nil)
}
