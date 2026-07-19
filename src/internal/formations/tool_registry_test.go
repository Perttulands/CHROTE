package formations

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolProfileDescriptorTypesAreClosedDataOnlyShapes(t *testing.T) {
	assertToolRegistryStructFields(t, reflect.TypeOf(ToolProfileDescriptor{}), []toolRegistryStructField{
		{name: "ProfileID", fieldType: reflect.TypeOf(""), jsonTag: "profileId"},
		{name: "ProfileVersion", fieldType: reflect.TypeOf(""), jsonTag: "profileVersion"},
		{name: "DisplayName", fieldType: reflect.TypeOf(""), jsonTag: "displayName"},
		{name: "Ports", fieldType: reflect.TypeOf([]ToolPortDescriptor{}), jsonTag: "ports"},
		{name: "Parameters", fieldType: reflect.TypeOf([]ToolParameterSpec{}), jsonTag: "parameters"},
	})
	assertToolRegistryStructFields(t, reflect.TypeOf(ToolPortDescriptor{}), []toolRegistryStructField{
		{name: "Name", fieldType: reflect.TypeOf(""), jsonTag: "name"},
		{name: "Label", fieldType: reflect.TypeOf(""), jsonTag: "label"},
		{name: "Direction", fieldType: reflect.TypeOf(""), jsonTag: "direction"},
		{name: "Kind", fieldType: reflect.TypeOf(""), jsonTag: "kind"},
		{name: "AcceptedMediaTypes", fieldType: reflect.TypeOf([]string{}), jsonTag: "acceptedMediaTypes"},
		{name: "Required", fieldType: reflect.TypeOf((*bool)(nil)), jsonTag: "required,omitempty"},
		{name: "Role", fieldType: reflect.TypeOf((*string)(nil)), jsonTag: "role,omitempty"},
	})
	assertToolRegistryStructFields(t, reflect.TypeOf(ToolParameterSpec{}), []toolRegistryStructField{
		{name: "Name", fieldType: reflect.TypeOf(""), jsonTag: "name"},
		{name: "Label", fieldType: reflect.TypeOf(""), jsonTag: "label"},
		{name: "Type", fieldType: reflect.TypeOf(""), jsonTag: "type"},
		{name: "Required", fieldType: reflect.TypeOf(false), jsonTag: "required"},
		{name: "Enum", fieldType: reflect.TypeOf([]string{}), jsonTag: "enum,omitempty"},
		{name: "MinBytes", fieldType: reflect.TypeOf((*int)(nil)), jsonTag: "minBytes,omitempty"},
		{name: "MaxBytes", fieldType: reflect.TypeOf((*int)(nil)), jsonTag: "maxBytes,omitempty"},
		{name: "Minimum", fieldType: reflect.TypeOf((*int64)(nil)), jsonTag: "minimum,omitempty"},
		{name: "Maximum", fieldType: reflect.TypeOf((*int64)(nil)), jsonTag: "maximum,omitempty"},
	})

	for _, descriptorType := range []reflect.Type{
		reflect.TypeOf(ToolProfileDescriptor{}),
		reflect.TypeOf(ToolPortDescriptor{}),
		reflect.TypeOf(ToolParameterSpec{}),
	} {
		assertToolRegistryTypeIsDataOnly(t, descriptorType, descriptorType.Name())
		assertToolRegistryTypeHasNoCustomSerialization(t, descriptorType)
	}
}

func TestToolProfileRegistryContainsOnlyFrozenJSONNormalizeVersionOne(t *testing.T) {
	first := ListToolProfileDescriptors()
	second := ListToolProfileDescriptors()
	if len(first) != 1 {
		t.Fatalf("Tool profile descriptor count = %d, want sole json.normalize@1", len(first))
	}

	want := frozenJSONNormalizeToolProfileDescriptor()
	if !reflect.DeepEqual(first[0], want) {
		t.Fatalf("sole Tool profile descriptor = %#v, want %#v", first[0], want)
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first Tool profile descriptor listing: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second Tool profile descriptor listing: %v", err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("Tool profile descriptor listing is not deterministic:\nfirst  %s\nsecond %s", firstJSON, secondJSON)
	}

	wantJSON := `[{"profileId":"json.normalize","profileVersion":"1","displayName":"Normalize JSON","ports":[{"name":"input","label":"Report","direction":"input","kind":"work","acceptedMediaTypes":["application/json"],"required":true,"role":"data"},{"name":"output","label":"Normalized report","direction":"output","kind":"work","acceptedMediaTypes":["application/json"]}],"parameters":[{"name":"mode","label":"Mode","type":"string","required":true,"enum":["strict"],"minBytes":6,"maxBytes":6}]}]`
	if string(firstJSON) != wantJSON {
		t.Fatalf("Tool profile descriptor JSON = %s, want exact closed catalog %s", firstJSON, wantJSON)
	}
}

func TestToolProfileRegistryLookupUsesExactTupleWithoutFallback(t *testing.T) {
	want := frozenJSONNormalizeToolProfileDescriptor()
	got, ok := LookupToolProfileDescriptor("json.normalize", "1")
	if !ok {
		t.Fatal("exact json.normalize@1 registry lookup reported missing")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exact registry lookup = %#v, want %#v", got, want)
	}

	misses := []struct {
		name           string
		profileID      string
		profileVersion string
	}{
		{name: "empty profile", profileVersion: "1"},
		{name: "case changed profile", profileID: "JSON.normalize", profileVersion: "1"},
		{name: "partial profile", profileID: "json", profileVersion: "1"},
		{name: "empty version", profileID: "json.normalize"},
		{name: "version alias", profileID: "json.normalize", profileVersion: "01"},
		{name: "version suffix", profileID: "json.normalize", profileVersion: "1.0"},
		{name: "latest selection", profileID: "json.normalize", profileVersion: "latest"},
		{name: "version whitespace", profileID: "json.normalize", profileVersion: "1 "},
	}
	for _, tt := range misses {
		t.Run(tt.name, func(t *testing.T) {
			if descriptor, found := LookupToolProfileDescriptor(tt.profileID, tt.profileVersion); found {
				t.Fatalf("non-exact registry lookup %q@%q resolved to %#v", tt.profileID, tt.profileVersion, descriptor)
			}
		})
	}
}

func TestToolProfileRegistryReturnsDeepFreshCopies(t *testing.T) {
	want := frozenJSONNormalizeToolProfileDescriptor()

	listed := ListToolProfileDescriptors()
	if len(listed) != 1 {
		t.Fatalf("Tool profile descriptor count = %d, want 1", len(listed))
	}
	mutateToolProfileDescriptorCopy(t, &listed[0])
	assertFrozenJSONNormalizeDescriptor(t, ListToolProfileDescriptors(), "listing after returned list mutation")

	lookedUp, ok := LookupToolProfileDescriptor("json.normalize", "1")
	if !ok {
		t.Fatal("exact json.normalize@1 registry lookup reported missing")
	}
	mutateToolProfileDescriptorCopy(t, &lookedUp)
	assertFrozenJSONNormalizeDescriptor(t, ListToolProfileDescriptors(), "listing after returned lookup mutation")
	lookedUpAgain, ok := LookupToolProfileDescriptor("json.normalize", "1")
	if !ok {
		t.Fatal("exact json.normalize@1 registry lookup disappeared after returned lookup mutation")
	}
	if !reflect.DeepEqual(lookedUpAgain, want) {
		t.Fatalf("lookup after returned lookup mutation = %#v, want fresh frozen copy %#v", lookedUpAgain, want)
	}

	source := []ToolProfileDescriptor{want}
	compiled, err := compileToolProfileRegistry(source)
	if err != nil {
		t.Fatalf("compile frozen Tool profile descriptor: %v", err)
	}
	mutateToolProfileDescriptorCopy(t, &source[0])
	compiledList := compiled.list()
	assertFrozenJSONNormalizeDescriptor(t, compiledList, "compiled registry after source mutation")
	mutateToolProfileDescriptorCopy(t, &compiledList[0])
	assertFrozenJSONNormalizeDescriptor(t, compiled.list(), "compiled registry after returned list mutation")
}

func TestCompileToolProfileRegistryRejectsInvalidProfileIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolProfileDescriptor)
	}{
		{name: "empty profile id", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "" }},
		{name: "profile without namespace", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json" }},
		{name: "uppercase profile id", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "JSON.normalize" }},
		{name: "uppercase profile segment", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json.Normalize" }},
		{name: "empty profile segment", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json..normalize" }},
		{name: "profile underscore", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json_normalize.value" }},
		{name: "profile non ASCII", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json.nörmalize" }},
		{name: "profile over 128 bytes", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "a." + strings.Repeat("b", 127) }},
		{name: "empty version", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "" }},
		{name: "version leading punctuation", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "-1" }},
		{name: "version whitespace", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "1 stable" }},
		{name: "version slash", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "1/stable" }},
		{name: "version non ASCII", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "vérsion" }},
		{name: "version over 64 bytes", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileVersion = "A" + strings.Repeat("b", 64) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := frozenJSONNormalizeToolProfileDescriptor()
			tt.mutate(&descriptor)
			if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor}); err == nil {
				t.Fatalf("registry compiler accepted invalid profile identity %#v", descriptor)
			}
		})
	}

	descriptor := frozenJSONNormalizeToolProfileDescriptor()
	if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor, descriptor}); err == nil {
		t.Fatal("registry compiler accepted duplicate exact profile tuple")
	}
}

func TestCompileToolProfileRegistryRejectsInvalidPortDescriptors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolProfileDescriptor)
	}{
		{name: "empty port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "" }},
		{name: "uppercase port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "Input" }},
		{name: "hyphenated port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "report-input" }},
		{name: "port name over 64 bytes", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "a" + strings.Repeat("b", 64) }},
		{name: "duplicate semantic port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[1].Name = descriptor.Ports[0].Name }},
		{name: "invalid direction", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Direction = "sideways" }},
		{name: "invalid kind", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Kind = "gate_feedback" }},
		{name: "empty media set", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].AcceptedMediaTypes = nil }},
		{name: "duplicate media type", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Ports[0].AcceptedMediaTypes = []string{"application/json", "application/json"}
		}},
		{name: "unapproved media type", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Ports[0].AcceptedMediaTypes = []string{"application/octet-stream"}
		}},
		{name: "output required presence", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[1].Required = toolRegistryBoolPointer(false) }},
		{name: "output role presence", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[1].Role = toolRegistryStringPointer("data") }},
		{name: "invalid input role", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Ports[0].Role = toolRegistryStringPointer("retry_control")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := frozenJSONNormalizeToolProfileDescriptor()
			tt.mutate(&descriptor)
			if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor}); err == nil {
				t.Fatalf("registry compiler accepted invalid port descriptor %#v", descriptor.Ports)
			}
		})
	}
}

func TestCompileToolProfileRegistryRejectsInvalidParameterDescriptors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolProfileDescriptor)
	}{
		{name: "empty parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "" }},
		{name: "uppercase parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "Mode" }},
		{name: "hyphenated parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "strict-mode" }},
		{name: "parameter name over 64 bytes", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "a" + strings.Repeat("b", 64) }},
		{name: "duplicate parameter name", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters = append(descriptor.Parameters, descriptor.Parameters[0])
		}},
		{name: "unknown parameter type", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Type = "number" }},
		{name: "string with integer minimum", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters[0].Minimum = toolRegistryInt64Pointer(0)
		}},
		{name: "string with integer maximum", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters[0].Maximum = toolRegistryInt64Pointer(1)
		}},
		{name: "negative string minimum bytes", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters[0].MinBytes = toolRegistryIntPointer(-1)
		}},
		{name: "inverted string byte bounds", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters[0].MinBytes = toolRegistryIntPointer(7)
			descriptor.Parameters[0].MaxBytes = toolRegistryIntPointer(6)
		}},
		{name: "duplicate enum value", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Enum = []string{"strict", "strict"} }},
		{name: "enum value below byte bound", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Enum = []string{"short"} }},
		{name: "enum value with NUL", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Enum = []string{"stri\x00ct"} }},
		{name: "enum value with invalid UTF-8", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters[0].Enum = []string{string([]byte{0xff})}
		}},
		{name: "integer with enum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Enum = []string{"1"}
		}},
		{name: "integer with string byte bound", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].MinBytes = toolRegistryIntPointer(1)
		}},
		{name: "integer missing minimum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Minimum = nil
		}},
		{name: "integer missing maximum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Maximum = nil
		}},
		{name: "inverted integer bounds", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Minimum = toolRegistryInt64Pointer(2)
			descriptor.Parameters[0].Maximum = toolRegistryInt64Pointer(1)
		}},
		{name: "unsafe integer minimum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Minimum = toolRegistryInt64Pointer(-9007199254740992)
		}},
		{name: "unsafe integer maximum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Maximum = toolRegistryInt64Pointer(9007199254740992)
		}},
		{name: "boolean with enum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Enum = []string{"true"}
		}},
		{name: "boolean with byte bound", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].MinBytes = toolRegistryIntPointer(1)
		}},
		{name: "boolean with integer bound", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Minimum = toolRegistryInt64Pointer(0)
		}},
	}

	for _, forbidden := range []string{
		"client_secret",
		"credential_hint",
		"refresh_token",
		"password_hint",
		"passwd_file",
		"api_key",
		"apikey",
		"private_key",
		"access_key",
		"executable",
		"exec",
		"command",
		"cmd",
		"argv",
		"shell",
		"cwd",
		"env",
		"environment",
		"path",
		"bundle",
		"limits",
		"network",
		"effects",
	} {
		forbidden := forbidden
		tests = append(tests, struct {
			name   string
			mutate func(*ToolProfileDescriptor)
		}{
			name: "forbidden parameter name " + forbidden,
			mutate: func(descriptor *ToolProfileDescriptor) {
				descriptor.Parameters[0].Name = forbidden
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := frozenJSONNormalizeToolProfileDescriptor()
			tt.mutate(&descriptor)
			if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor}); err == nil {
				t.Fatalf("registry compiler accepted invalid parameter descriptor %#v", descriptor.Parameters)
			}
		})
	}
}

type toolRegistryStructField struct {
	name      string
	fieldType reflect.Type
	jsonTag   string
}

func assertToolRegistryStructFields(t *testing.T, structType reflect.Type, want []toolRegistryStructField) {
	t.Helper()
	if structType.Kind() != reflect.Struct {
		t.Fatalf("Tool registry shape %s kind = %s, want struct", structType, structType.Kind())
	}
	if structType.NumField() != len(want) {
		t.Fatalf("Tool registry shape %s field count = %d, want exact closed count %d", structType, structType.NumField(), len(want))
	}
	for index, expected := range want {
		field := structType.Field(index)
		if field.Name != expected.name || field.Type != expected.fieldType || string(field.Tag) != `json:"`+expected.jsonTag+`"` {
			t.Fatalf("Tool registry shape %s field %d = %s %s %q, want %s %s %q", structType, index, field.Name, field.Type, field.Tag, expected.name, expected.fieldType, `json:"`+expected.jsonTag+`"`)
		}
	}
}

func assertToolRegistryTypeIsDataOnly(t *testing.T, valueType reflect.Type, path string) {
	t.Helper()
	switch valueType.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		assertToolRegistryTypeIsDataOnly(t, valueType.Elem(), path+"[]")
	case reflect.Struct:
		for index := 0; index < valueType.NumField(); index++ {
			field := valueType.Field(index)
			assertToolRegistryTypeIsDataOnly(t, field.Type, path+"."+field.Name)
		}
	case reflect.Interface, reflect.Func, reflect.Chan, reflect.Map, reflect.UnsafePointer:
		t.Fatalf("Tool registry shape %s contains non-data field kind %s", path, valueType.Kind())
	}
}

func assertToolRegistryTypeHasNoCustomSerialization(t *testing.T, valueType reflect.Type) {
	t.Helper()
	jsonMarshaler := reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshaler := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	for _, candidate := range []reflect.Type{valueType, reflect.PointerTo(valueType)} {
		if candidate.Implements(jsonMarshaler) {
			t.Fatalf("closed Tool registry shape %s implements json.Marshaler", candidate)
		}
		if candidate.Implements(textMarshaler) {
			t.Fatalf("closed Tool registry shape %s implements encoding.TextMarshaler", candidate)
		}
	}
}

func frozenJSONNormalizeToolProfileDescriptor() ToolProfileDescriptor {
	return ToolProfileDescriptor{
		ProfileID:      "json.normalize",
		ProfileVersion: "1",
		DisplayName:    "Normalize JSON",
		Ports: []ToolPortDescriptor{
			{
				Name:               "input",
				Label:              "Report",
				Direction:          "input",
				Kind:               "work",
				AcceptedMediaTypes: []string{"application/json"},
				Required:           toolRegistryBoolPointer(true),
				Role:               toolRegistryStringPointer("data"),
			},
			{
				Name:               "output",
				Label:              "Normalized report",
				Direction:          "output",
				Kind:               "work",
				AcceptedMediaTypes: []string{"application/json"},
			},
		},
		Parameters: []ToolParameterSpec{
			{
				Name:     "mode",
				Label:    "Mode",
				Type:     "string",
				Required: true,
				Enum:     []string{"strict"},
				MinBytes: toolRegistryIntPointer(6),
				MaxBytes: toolRegistryIntPointer(6),
			},
		},
	}
}

func mutateToolProfileDescriptorCopy(t *testing.T, descriptor *ToolProfileDescriptor) {
	t.Helper()
	if len(descriptor.Ports) < 2 || descriptor.Ports[0].Required == nil || descriptor.Ports[0].Role == nil || len(descriptor.Ports[0].AcceptedMediaTypes) == 0 {
		t.Fatalf("copy-mutation fixture has incomplete ports: %#v", descriptor.Ports)
	}
	if len(descriptor.Parameters) == 0 || len(descriptor.Parameters[0].Enum) == 0 || descriptor.Parameters[0].MinBytes == nil || descriptor.Parameters[0].MaxBytes == nil {
		t.Fatalf("copy-mutation fixture has incomplete parameters: %#v", descriptor.Parameters)
	}

	descriptor.ProfileID = "mutated.profile"
	descriptor.ProfileVersion = "mutated"
	descriptor.DisplayName = "Mutated"
	descriptor.Ports[0].Name = "mutated_input"
	descriptor.Ports[0].AcceptedMediaTypes[0] = "mutated/media"
	*descriptor.Ports[0].Required = false
	*descriptor.Ports[0].Role = "mutated"
	descriptor.Ports[1].Label = "Mutated output"
	descriptor.Parameters[0].Name = "mutated_mode"
	descriptor.Parameters[0].Enum[0] = "mutated"
	*descriptor.Parameters[0].MinBytes = 1
	*descriptor.Parameters[0].MaxBytes = 99
}

func assertFrozenJSONNormalizeDescriptor(t *testing.T, descriptors []ToolProfileDescriptor, context string) {
	t.Helper()
	if len(descriptors) != 1 {
		t.Fatalf("%s descriptor count = %d, want 1", context, len(descriptors))
	}
	if want := frozenJSONNormalizeToolProfileDescriptor(); !reflect.DeepEqual(descriptors[0], want) {
		t.Fatalf("%s = %#v, want fresh frozen copy %#v", context, descriptors[0], want)
	}
}

func makeToolRegistryIntegerParameter(parameter *ToolParameterSpec) {
	parameter.Type = "integer"
	parameter.Enum = nil
	parameter.MinBytes = nil
	parameter.MaxBytes = nil
	parameter.Minimum = toolRegistryInt64Pointer(-1)
	parameter.Maximum = toolRegistryInt64Pointer(1)
}

func makeToolRegistryBooleanParameter(parameter *ToolParameterSpec) {
	parameter.Type = "boolean"
	parameter.Enum = nil
	parameter.MinBytes = nil
	parameter.MaxBytes = nil
	parameter.Minimum = nil
	parameter.Maximum = nil
}

func toolRegistryBoolPointer(value bool) *bool {
	return &value
}

func toolRegistryStringPointer(value string) *string {
	return &value
}

func toolRegistryIntPointer(value int) *int {
	return &value
}

func toolRegistryInt64Pointer(value int64) *int64 {
	return &value
}
