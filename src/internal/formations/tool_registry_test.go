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
		{name: "ProfileID", logicalType: "string", jsonTag: "profileId"},
		{name: "ProfileVersion", logicalType: "string", jsonTag: "profileVersion"},
		{name: "DisplayName", logicalType: "string", jsonTag: "displayName"},
		{name: "Ports", logicalType: "slice:ToolPortDescriptor", jsonTag: "ports"},
		{name: "Parameters", logicalType: "slice:ToolParameterSpec", jsonTag: "parameters"},
	})
	assertToolRegistryStructFields(t, reflect.TypeOf(ToolPortDescriptor{}), []toolRegistryStructField{
		{name: "Name", logicalType: "string", jsonTag: "name"},
		{name: "Label", logicalType: "string", jsonTag: "label"},
		{name: "Direction", logicalType: "string", jsonTag: "direction"},
		{name: "Kind", logicalType: "string", jsonTag: "kind"},
		{name: "AcceptedMediaTypes", logicalType: "slice:string", jsonTag: "acceptedMediaTypes"},
		{name: "Required", logicalType: "pointer:bool", jsonTag: "required,omitempty"},
		{name: "Role", logicalType: "pointer:string", jsonTag: "role,omitempty"},
	})
	assertToolRegistryStructFields(t, reflect.TypeOf(ToolParameterSpec{}), []toolRegistryStructField{
		{name: "Name", logicalType: "string", jsonTag: "name"},
		{name: "Label", logicalType: "string", jsonTag: "label"},
		{name: "Type", logicalType: "string", jsonTag: "type"},
		{name: "Required", logicalType: "bool", jsonTag: "required"},
		{name: "Enum", logicalType: "slice:string", jsonTag: "enum,omitempty"},
		{name: "MinBytes", logicalType: "pointer:signed-integer", jsonTag: "minBytes,omitempty"},
		{name: "MaxBytes", logicalType: "pointer:signed-integer", jsonTag: "maxBytes,omitempty"},
		{name: "Minimum", logicalType: "pointer:signed-integer", jsonTag: "minimum,omitempty"},
		{name: "Maximum", logicalType: "pointer:signed-integer", jsonTag: "maximum,omitempty"},
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

	want := frozenJSONNormalizeToolProfileDescriptor(t)
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
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Tool profile descriptor listing is not deterministic:\nfirst  %#v\nsecond %#v", first, second)
	}

	wantJSON := `[{"profileId":"json.normalize","profileVersion":"1","displayName":"Normalize JSON","ports":[{"name":"input","label":"Report","direction":"input","kind":"work","acceptedMediaTypes":["application/json"],"required":true,"role":"data"},{"name":"output","label":"Normalized report","direction":"output","kind":"work","acceptedMediaTypes":["application/json"]}],"parameters":[{"name":"mode","label":"Mode","type":"string","required":true,"enum":["strict"],"minBytes":6,"maxBytes":6}]}]`
	var gotProjection, secondProjection, wantProjection any
	if err := json.Unmarshal(firstJSON, &gotProjection); err != nil {
		t.Fatalf("decode Tool profile descriptor listing: %v", err)
	}
	if err := json.Unmarshal(secondJSON, &secondProjection); err != nil {
		t.Fatalf("decode repeated Tool profile descriptor listing: %v", err)
	}
	if !reflect.DeepEqual(gotProjection, secondProjection) {
		t.Fatalf("Tool profile descriptor JSON projection is not deterministic:\nfirst  %#v\nsecond %#v", gotProjection, secondProjection)
	}
	if err := json.Unmarshal([]byte(wantJSON), &wantProjection); err != nil {
		t.Fatalf("decode expected Tool profile descriptor listing: %v", err)
	}
	if !reflect.DeepEqual(gotProjection, wantProjection) {
		t.Fatalf("Tool profile descriptor JSON = %s, want exact closed catalog %s", firstJSON, wantJSON)
	}
}

func TestToolProfileRegistryLookupUsesExactTupleWithoutFallback(t *testing.T) {
	want := frozenJSONNormalizeToolProfileDescriptor(t)
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

func TestCompileToolProfileRegistryAcceptsContractValidGenericDescriptors(t *testing.T) {
	jsonNormalize := frozenJSONNormalizeToolProfileDescriptor(t)
	boundary := contractValidBoundaryToolProfileDescriptor()
	compiled, err := compileToolProfileRegistry([]ToolProfileDescriptor{jsonNormalize, boundary})
	if err != nil {
		t.Fatalf("compile two distinct contract-valid Tool profiles: %v", err)
	}

	listed := compiled.list()
	if len(listed) != 2 {
		t.Fatalf("compiled Tool profile count = %d, want both distinct descriptors", len(listed))
	}
	seen := make(map[string]bool, len(listed))
	for _, descriptor := range listed {
		seen[descriptor.ProfileID+"@"+descriptor.ProfileVersion] = true
	}
	for _, descriptor := range []ToolProfileDescriptor{jsonNormalize, boundary} {
		key := descriptor.ProfileID + "@" + descriptor.ProfileVersion
		if !seen[key] {
			t.Fatalf("compiled Tool profile listing omitted valid tuple %s", key)
		}
		got, ok := compiled.lookup(descriptor.ProfileID, descriptor.ProfileVersion)
		if !ok {
			t.Fatalf("compiled Tool profile lookup omitted valid tuple %s", key)
		}
		if !reflect.DeepEqual(got, descriptor) {
			t.Fatalf("compiled Tool profile %s = %#v, want %#v", key, got, descriptor)
		}
	}
}

func TestToolProfileRegistryReturnsDeepFreshCopies(t *testing.T) {
	want := frozenJSONNormalizeToolProfileDescriptor(t)

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

	source := []ToolProfileDescriptor{frozenJSONNormalizeToolProfileDescriptor(t)}
	compiled, err := compileToolProfileRegistry(source)
	if err != nil {
		t.Fatalf("compile frozen Tool profile descriptor: %v", err)
	}
	mutateToolProfileDescriptorCopy(t, &source[0])
	compiledList := compiled.list()
	assertFrozenJSONNormalizeDescriptor(t, compiledList, "compiled registry after source mutation")
	compiledLookup, ok := compiled.lookup("json.normalize", "1")
	if !ok {
		t.Fatal("freshly compiled registry lost exact json.normalize@1 after source mutation")
	}
	if !reflect.DeepEqual(compiledLookup, want) {
		t.Fatalf("freshly compiled lookup after source mutation = %#v, want %#v", compiledLookup, want)
	}
	mutateToolProfileDescriptorCopy(t, &compiledLookup)
	compiledLookupAgain, ok := compiled.lookup("json.normalize", "1")
	if !ok {
		t.Fatal("freshly compiled registry lost exact json.normalize@1 after returned lookup mutation")
	}
	if !reflect.DeepEqual(compiledLookupAgain, want) {
		t.Fatalf("freshly compiled lookup after returned lookup mutation = %#v, want fresh copy %#v", compiledLookupAgain, want)
	}
	mutateToolProfileDescriptorCopy(t, &compiledList[0])
	assertFrozenJSONNormalizeDescriptor(t, compiled.list(), "compiled registry after returned list mutation")

	boundaryWant := contractValidBoundaryToolProfileDescriptor()
	boundarySource := []ToolProfileDescriptor{contractValidBoundaryToolProfileDescriptor()}
	boundaryCompiled, err := compileToolProfileRegistry(boundarySource)
	if err != nil {
		t.Fatalf("compile contract-valid boundary Tool profile descriptor: %v", err)
	}
	mutateBoundaryToolProfileDescriptorCopy(t, &boundarySource[0])
	boundaryLookup, ok := boundaryCompiled.lookup(boundaryWant.ProfileID, boundaryWant.ProfileVersion)
	if !ok || !reflect.DeepEqual(boundaryLookup, boundaryWant) {
		t.Fatalf("boundary lookup after compiler-source mutation = %#v/%v, want fresh copy %#v/true", boundaryLookup, ok, boundaryWant)
	}
	mutateBoundaryToolProfileDescriptorCopy(t, &boundaryLookup)
	boundaryLookupAgain, ok := boundaryCompiled.lookup(boundaryWant.ProfileID, boundaryWant.ProfileVersion)
	if !ok || !reflect.DeepEqual(boundaryLookupAgain, boundaryWant) {
		t.Fatalf("boundary lookup after returned-lookup mutation = %#v/%v, want fresh copy %#v/true", boundaryLookupAgain, ok, boundaryWant)
	}
}

func TestCompileToolProfileRegistryRejectsInvalidProfileIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolProfileDescriptor)
	}{
		{name: "empty profile id", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "" }},
		{name: "profile without namespace", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json" }},
		{name: "profile leading digit", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "1json.normalize" }},
		{name: "profile segment leading digit", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.ProfileID = "json.1normalize" }},
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
			descriptor := frozenJSONNormalizeToolProfileDescriptor(t)
			tt.mutate(&descriptor)
			if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor}); err == nil {
				t.Fatalf("registry compiler accepted invalid profile identity %#v", descriptor)
			}
		})
	}

	descriptor := frozenJSONNormalizeToolProfileDescriptor(t)
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
		{name: "leading digit port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "1input" }},
		{name: "leading underscore port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "_input" }},
		{name: "uppercase port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "Input" }},
		{name: "hyphenated port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "report-input" }},
		{name: "non ASCII port name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Ports[0].Name = "inpuţ" }},
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
	for _, forbidden := range toolRegistryForbiddenMachineNames() {
		forbidden := forbidden
		tests = append(tests, struct {
			name   string
			mutate func(*ToolProfileDescriptor)
		}{
			name: "forbidden port name " + forbidden,
			mutate: func(descriptor *ToolProfileDescriptor) {
				descriptor.Ports[0].Name = forbidden
			},
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := frozenJSONNormalizeToolProfileDescriptor(t)
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
		{name: "leading digit parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "1mode" }},
		{name: "leading underscore parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "_mode" }},
		{name: "uppercase parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "Mode" }},
		{name: "hyphenated parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "strict-mode" }},
		{name: "non ASCII parameter name", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "møde" }},
		{name: "parameter name over 64 bytes", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Name = "a" + strings.Repeat("b", 64) }},
		{name: "duplicate parameter name", mutate: func(descriptor *ToolProfileDescriptor) {
			descriptor.Parameters = append(descriptor.Parameters, descriptor.Parameters[0])
		}},
		{name: "unknown parameter type", mutate: func(descriptor *ToolProfileDescriptor) { descriptor.Parameters[0].Type = "number" }},
		{name: "string with integer minimum", mutate: func(descriptor *ToolProfileDescriptor) {
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Minimum", 0)
		}},
		{name: "string with integer maximum", mutate: func(descriptor *ToolProfileDescriptor) {
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Maximum", 1)
		}},
		{name: "negative string minimum bytes", mutate: func(descriptor *ToolProfileDescriptor) {
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "MinBytes", -1)
		}},
		{name: "inverted string byte bounds", mutate: func(descriptor *ToolProfileDescriptor) {
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "MinBytes", 7)
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "MaxBytes", 6)
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
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "MinBytes", 1)
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
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Minimum", 2)
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Maximum", 1)
		}},
		{name: "unsafe integer minimum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Minimum", -9007199254740992)
		}},
		{name: "unsafe integer maximum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryIntegerParameter(&descriptor.Parameters[0])
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Maximum", 9007199254740992)
		}},
		{name: "boolean with enum", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			descriptor.Parameters[0].Enum = []string{"true"}
		}},
		{name: "boolean with byte bound", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "MinBytes", 1)
		}},
		{name: "boolean with integer bound", mutate: func(descriptor *ToolProfileDescriptor) {
			makeToolRegistryBooleanParameter(&descriptor.Parameters[0])
			setToolRegistryOptionalInteger(&descriptor.Parameters[0], "Minimum", 0)
		}},
	}

	for _, forbidden := range toolRegistryForbiddenMachineNames() {
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
			descriptor := frozenJSONNormalizeToolProfileDescriptor(t)
			tt.mutate(&descriptor)
			if _, err := compileToolProfileRegistry([]ToolProfileDescriptor{descriptor}); err == nil {
				t.Fatalf("registry compiler accepted invalid parameter descriptor %#v", descriptor.Parameters)
			}
		})
	}
}

type toolRegistryStructField struct {
	name        string
	logicalType string
	jsonTag     string
}

func assertToolRegistryStructFields(t *testing.T, structType reflect.Type, want []toolRegistryStructField) {
	t.Helper()
	if structType.Kind() != reflect.Struct {
		t.Fatalf("Tool registry shape %s kind = %s, want struct", structType, structType.Kind())
	}
	if structType.NumField() != len(want) {
		t.Fatalf("Tool registry shape %s field count = %d, want exact closed count %d", structType, structType.NumField(), len(want))
	}
	for _, expected := range want {
		field, ok := structType.FieldByName(expected.name)
		if !ok {
			t.Fatalf("Tool registry shape %s is missing logical field %s", structType, expected.name)
		}
		if string(field.Tag) != `json:"`+expected.jsonTag+`"` {
			t.Fatalf("Tool registry shape %s field %s tag = %q, want %q", structType, field.Name, field.Tag, `json:"`+expected.jsonTag+`"`)
		}
		if !toolRegistryFieldMatchesLogicalType(field.Type, expected.logicalType) {
			t.Fatalf("Tool registry shape %s field %s type = %s, want logical type %s", structType, field.Name, field.Type, expected.logicalType)
		}
	}
}

func toolRegistryFieldMatchesLogicalType(fieldType reflect.Type, logicalType string) bool {
	switch logicalType {
	case "string":
		return fieldType.Kind() == reflect.String
	case "bool":
		return fieldType.Kind() == reflect.Bool
	case "slice:string":
		return fieldType.Kind() == reflect.Slice && fieldType.Elem().Kind() == reflect.String
	case "slice:ToolPortDescriptor":
		return fieldType.Kind() == reflect.Slice && fieldType.Elem().Name() == "ToolPortDescriptor"
	case "slice:ToolParameterSpec":
		return fieldType.Kind() == reflect.Slice && fieldType.Elem().Name() == "ToolParameterSpec"
	case "pointer:bool":
		return fieldType.Kind() == reflect.Pointer && fieldType.Elem().Kind() == reflect.Bool
	case "pointer:string":
		return fieldType.Kind() == reflect.Pointer && fieldType.Elem().Kind() == reflect.String
	case "pointer:signed-integer":
		if fieldType.Kind() != reflect.Pointer {
			return false
		}
		switch fieldType.Elem().Kind() {
		case reflect.Int, reflect.Int64:
			return true
		default:
			return false
		}
	default:
		return false
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
	jsonUnmarshaler := reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textMarshaler := reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshaler := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	for _, candidate := range []reflect.Type{valueType, reflect.PointerTo(valueType)} {
		if candidate.Implements(jsonMarshaler) {
			t.Fatalf("closed Tool registry shape %s implements json.Marshaler", candidate)
		}
		if candidate.Implements(jsonUnmarshaler) {
			t.Fatalf("closed Tool registry shape %s implements json.Unmarshaler", candidate)
		}
		if candidate.Implements(textMarshaler) {
			t.Fatalf("closed Tool registry shape %s implements encoding.TextMarshaler", candidate)
		}
		if candidate.Implements(textUnmarshaler) {
			t.Fatalf("closed Tool registry shape %s implements encoding.TextUnmarshaler", candidate)
		}
	}
}

func frozenJSONNormalizeToolProfileDescriptor(t *testing.T) ToolProfileDescriptor {
	t.Helper()
	const raw = `{"profileId":"json.normalize","profileVersion":"1","displayName":"Normalize JSON","ports":[{"name":"input","label":"Report","direction":"input","kind":"work","acceptedMediaTypes":["application/json"],"required":true,"role":"data"},{"name":"output","label":"Normalized report","direction":"output","kind":"work","acceptedMediaTypes":["application/json"]}],"parameters":[{"name":"mode","label":"Mode","type":"string","required":true,"enum":["strict"],"minBytes":6,"maxBytes":6}]}`
	var descriptor ToolProfileDescriptor
	if err := json.Unmarshal([]byte(raw), &descriptor); err != nil {
		t.Fatalf("decode frozen json.normalize@1 Tool profile descriptor: %v", err)
	}
	return descriptor
}

func contractValidBoundaryToolProfileDescriptor() ToolProfileDescriptor {
	boundaryName := "a" + strings.Repeat("b", 63)
	descriptor := ToolProfileDescriptor{
		ProfileID:      "a." + strings.Repeat("b", 126),
		ProfileVersion: "A._-" + strings.Repeat("b", 60),
		DisplayName:    "Boundary profile fixture",
		Ports: []ToolPortDescriptor{
			{
				Name:               boundaryName,
				Label:              "Source",
				Direction:          "input",
				Kind:               "work",
				AcceptedMediaTypes: []string{"text/plain", "text/markdown"},
			},
			{
				Name:               "optional_input",
				Label:              "Optional source",
				Direction:          "input",
				Kind:               "work",
				AcceptedMediaTypes: []string{"text/plain"},
				Required:           toolRegistryBoolPointer(false),
				Role:               toolRegistryStringPointer("data"),
			},
			{
				Name:               "output",
				Label:              "Result",
				Direction:          "output",
				Kind:               "work",
				AcceptedMediaTypes: []string{"text/plain", "text/markdown"},
			},
		},
		Parameters: []ToolParameterSpec{
			{Name: boundaryName, Label: "Unconstrained text", Type: "string", Required: false},
			{Name: "enum_mode", Label: "Enumerated text", Type: "string", Required: false, Enum: []string{"strict"}},
			{Name: "min_text", Label: "Minimum text", Type: "string", Required: false},
			{Name: "max_text", Label: "Maximum text", Type: "string", Required: false},
			{Name: "enabled", Label: "Enabled", Type: "boolean", Required: false},
			{Name: "limit", Label: "Limit", Type: "integer", Required: false},
		},
	}
	setToolRegistryOptionalInteger(&descriptor.Parameters[2], "MinBytes", 1)
	setToolRegistryOptionalInteger(&descriptor.Parameters[3], "MaxBytes", 8)
	setToolRegistryOptionalInteger(&descriptor.Parameters[5], "Minimum", -9007199254740991)
	setToolRegistryOptionalInteger(&descriptor.Parameters[5], "Maximum", 9007199254740991)
	return descriptor
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

func mutateBoundaryToolProfileDescriptorCopy(t *testing.T, descriptor *ToolProfileDescriptor) {
	t.Helper()
	if len(descriptor.Ports) < 3 || descriptor.Ports[1].Required == nil || descriptor.Ports[1].Role == nil || len(descriptor.Ports[1].AcceptedMediaTypes) == 0 {
		t.Fatalf("boundary copy-mutation fixture has incomplete ports: %#v", descriptor.Ports)
	}
	if len(descriptor.Parameters) < 6 || descriptor.Parameters[2].MinBytes == nil || descriptor.Parameters[3].MaxBytes == nil || descriptor.Parameters[5].Minimum == nil || descriptor.Parameters[5].Maximum == nil {
		t.Fatalf("boundary copy-mutation fixture has incomplete parameters: %#v", descriptor.Parameters)
	}
	descriptor.Ports[1].AcceptedMediaTypes[0] = "mutated/media"
	*descriptor.Ports[1].Required = true
	*descriptor.Ports[1].Role = "mutated"
	*descriptor.Parameters[2].MinBytes = 0
	*descriptor.Parameters[3].MaxBytes = 1
	*descriptor.Parameters[5].Minimum = 0
	*descriptor.Parameters[5].Maximum = 0
}

func assertFrozenJSONNormalizeDescriptor(t *testing.T, descriptors []ToolProfileDescriptor, context string) {
	t.Helper()
	if len(descriptors) != 1 {
		t.Fatalf("%s descriptor count = %d, want 1", context, len(descriptors))
	}
	if want := frozenJSONNormalizeToolProfileDescriptor(t); !reflect.DeepEqual(descriptors[0], want) {
		t.Fatalf("%s = %#v, want fresh frozen copy %#v", context, descriptors[0], want)
	}
}

func makeToolRegistryIntegerParameter(parameter *ToolParameterSpec) {
	parameter.Type = "integer"
	parameter.Enum = nil
	parameter.MinBytes = nil
	parameter.MaxBytes = nil
	setToolRegistryOptionalInteger(parameter, "Minimum", -1)
	setToolRegistryOptionalInteger(parameter, "Maximum", 1)
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

func setToolRegistryOptionalInteger(parameter *ToolParameterSpec, fieldName string, value int64) {
	field := reflect.ValueOf(parameter).Elem().FieldByName(fieldName)
	if !field.IsValid() || field.Kind() != reflect.Pointer {
		panic("Tool parameter field " + fieldName + " is not an optional integer")
	}
	integer := reflect.New(field.Type().Elem())
	integer.Elem().SetInt(value)
	field.Set(integer)
}

func toolRegistryForbiddenMachineNames() []string {
	return []string{
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
	}
}
