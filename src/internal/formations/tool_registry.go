package formations

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ToolProfileDescriptor struct {
	ProfileID      string               `json:"profileId"`
	ProfileVersion string               `json:"profileVersion"`
	DisplayName    string               `json:"displayName"`
	Ports          []ToolPortDescriptor `json:"ports"`
	Parameters     []ToolParameterSpec  `json:"parameters"`
}

type ToolPortDescriptor struct {
	Name               string   `json:"name"`
	Label              string   `json:"label"`
	Direction          string   `json:"direction"`
	Kind               string   `json:"kind"`
	AcceptedMediaTypes []string `json:"acceptedMediaTypes"`
	Required           *bool    `json:"required,omitempty"`
	Role               *string  `json:"role,omitempty"`
}

type ToolParameterSpec struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Enum     []string `json:"enum,omitempty"`
	MinBytes *int64   `json:"minBytes,omitempty"`
	MaxBytes *int64   `json:"maxBytes,omitempty"`
	Minimum  *int64   `json:"minimum,omitempty"`
	Maximum  *int64   `json:"maximum,omitempty"`
}

type toolProfileKey struct {
	profileID      string
	profileVersion string
}

type compiledToolProfileRegistry struct {
	descriptors []ToolProfileDescriptor
	byKey       map[toolProfileKey]ToolProfileDescriptor
}

var toolProfileRegistry = mustCompileToolProfileRegistry([]ToolProfileDescriptor{
	{
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
				Required:           toolDescriptorBool(true),
				Role:               toolDescriptorString("data"),
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
				MinBytes: toolDescriptorInteger(6),
				MaxBytes: toolDescriptorInteger(6),
			},
		},
	},
})

func ListToolProfileDescriptors() []ToolProfileDescriptor {
	return toolProfileRegistry.list()
}

func LookupToolProfileDescriptor(profileID, profileVersion string) (ToolProfileDescriptor, bool) {
	return toolProfileRegistry.lookup(profileID, profileVersion)
}

func compileToolProfileRegistry(descriptors []ToolProfileDescriptor) (compiledToolProfileRegistry, error) {
	compiled := compiledToolProfileRegistry{
		descriptors: make([]ToolProfileDescriptor, 0, len(descriptors)),
		byKey:       make(map[toolProfileKey]ToolProfileDescriptor, len(descriptors)),
	}
	for index, descriptor := range descriptors {
		if err := validateToolProfileDescriptor(descriptor); err != nil {
			return compiledToolProfileRegistry{}, fmt.Errorf("Tool profile descriptor %d: %w", index, err)
		}
		key := toolProfileKey{profileID: descriptor.ProfileID, profileVersion: descriptor.ProfileVersion}
		if _, exists := compiled.byKey[key]; exists {
			return compiledToolProfileRegistry{}, fmt.Errorf("duplicate Tool profile %q@%q", descriptor.ProfileID, descriptor.ProfileVersion)
		}
		copy := cloneToolProfileDescriptor(descriptor)
		compiled.descriptors = append(compiled.descriptors, copy)
		compiled.byKey[key] = copy
	}
	return compiled, nil
}

func mustCompileToolProfileRegistry(descriptors []ToolProfileDescriptor) compiledToolProfileRegistry {
	compiled, err := compileToolProfileRegistry(descriptors)
	if err != nil {
		panic("compile built-in Tool profile registry: " + err.Error())
	}
	return compiled
}

func (registry compiledToolProfileRegistry) list() []ToolProfileDescriptor {
	descriptors := make([]ToolProfileDescriptor, len(registry.descriptors))
	for index, descriptor := range registry.descriptors {
		descriptors[index] = cloneToolProfileDescriptor(descriptor)
	}
	return descriptors
}

func (registry compiledToolProfileRegistry) lookup(profileID, profileVersion string) (ToolProfileDescriptor, bool) {
	descriptor, ok := registry.byKey[toolProfileKey{profileID: profileID, profileVersion: profileVersion}]
	if !ok {
		return ToolProfileDescriptor{}, false
	}
	return cloneToolProfileDescriptor(descriptor), true
}

func validateToolProfileDescriptor(descriptor ToolProfileDescriptor) error {
	if !validToolProfileID(descriptor.ProfileID) {
		return fmt.Errorf("invalid profile id %q", descriptor.ProfileID)
	}
	if !validToolProfileVersion(descriptor.ProfileVersion) {
		return fmt.Errorf("invalid profile version %q", descriptor.ProfileVersion)
	}
	portNames := make(map[string]struct{}, len(descriptor.Ports))
	for index, port := range descriptor.Ports {
		if err := validateToolPortDescriptor(port); err != nil {
			return fmt.Errorf("port %d: %w", index, err)
		}
		if _, exists := portNames[port.Name]; exists {
			return fmt.Errorf("duplicate port name %q", port.Name)
		}
		portNames[port.Name] = struct{}{}
	}

	parameterNames := make(map[string]struct{}, len(descriptor.Parameters))
	for index, parameter := range descriptor.Parameters {
		if err := validateToolParameterSpec(parameter); err != nil {
			return fmt.Errorf("parameter %d: %w", index, err)
		}
		if _, exists := parameterNames[parameter.Name]; exists {
			return fmt.Errorf("duplicate parameter name %q", parameter.Name)
		}
		parameterNames[parameter.Name] = struct{}{}
	}
	return nil
}

func validateToolPortDescriptor(port ToolPortDescriptor) error {
	if !validToolMachineName(port.Name) || forbiddenToolMachineName(port.Name) {
		return fmt.Errorf("invalid port name %q", port.Name)
	}
	if port.Direction != "input" && port.Direction != "output" {
		return fmt.Errorf("invalid port direction %q", port.Direction)
	}
	if port.Kind != "work" {
		return fmt.Errorf("invalid port kind %q", port.Kind)
	}
	if err := validateToolMediaTypes(port.AcceptedMediaTypes); err != nil {
		return err
	}
	if port.Direction == "output" {
		if port.Required != nil || port.Role != nil {
			return fmt.Errorf("output port %q has input-only fields", port.Name)
		}
		return nil
	}
	if port.Role != nil && *port.Role != "data" {
		return fmt.Errorf("invalid input port role %q", *port.Role)
	}
	return nil
}

func validateToolMediaTypes(mediaTypes []string) error {
	if len(mediaTypes) == 0 {
		return fmt.Errorf("empty accepted media types")
	}
	seen := make(map[string]struct{}, len(mediaTypes))
	for _, mediaType := range mediaTypes {
		switch mediaType {
		case "text/plain", "text/markdown", "application/json":
		default:
			return fmt.Errorf("unsupported media type %q", mediaType)
		}
		if _, exists := seen[mediaType]; exists {
			return fmt.Errorf("duplicate media type %q", mediaType)
		}
		seen[mediaType] = struct{}{}
	}
	return nil
}

func validateToolParameterSpec(parameter ToolParameterSpec) error {
	if !validToolMachineName(parameter.Name) || forbiddenToolMachineName(parameter.Name) {
		return fmt.Errorf("invalid parameter name %q", parameter.Name)
	}
	switch parameter.Type {
	case "string":
		if parameter.Minimum != nil || parameter.Maximum != nil {
			return fmt.Errorf("string parameter %q has integer constraints", parameter.Name)
		}
		if parameter.MinBytes != nil && *parameter.MinBytes < 0 {
			return fmt.Errorf("string parameter %q has negative minimum byte length", parameter.Name)
		}
		if parameter.MaxBytes != nil && *parameter.MaxBytes < 0 {
			return fmt.Errorf("string parameter %q has negative maximum byte length", parameter.Name)
		}
		if parameter.MinBytes != nil && parameter.MaxBytes != nil && *parameter.MinBytes > *parameter.MaxBytes {
			return fmt.Errorf("string parameter %q has inverted byte bounds", parameter.Name)
		}
		if err := validateToolParameterEnum(parameter); err != nil {
			return err
		}
	case "integer":
		if parameter.Enum != nil || parameter.MinBytes != nil || parameter.MaxBytes != nil {
			return fmt.Errorf("integer parameter %q has string constraints", parameter.Name)
		}
		if parameter.Minimum == nil || parameter.Maximum == nil {
			return fmt.Errorf("integer parameter %q is missing bounds", parameter.Name)
		}
		if *parameter.Minimum < -maxToolParameterInteger || *parameter.Maximum > maxToolParameterInteger {
			return fmt.Errorf("integer parameter %q exceeds JSON-safe bounds", parameter.Name)
		}
		if *parameter.Minimum > *parameter.Maximum {
			return fmt.Errorf("integer parameter %q has inverted bounds", parameter.Name)
		}
	case "boolean":
		if parameter.Enum != nil || parameter.MinBytes != nil || parameter.MaxBytes != nil || parameter.Minimum != nil || parameter.Maximum != nil {
			return fmt.Errorf("boolean parameter %q has constraints", parameter.Name)
		}
	default:
		return fmt.Errorf("invalid parameter type %q", parameter.Type)
	}
	return nil
}

func validateToolParameterEnum(parameter ToolParameterSpec) error {
	seen := make(map[string]struct{}, len(parameter.Enum))
	for _, value := range parameter.Enum {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("parameter %q has invalid enum value", parameter.Name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("parameter %q has duplicate enum value", parameter.Name)
		}
		seen[value] = struct{}{}
		valueBytes := int64(len(value))
		if parameter.MinBytes != nil && valueBytes < *parameter.MinBytes {
			return fmt.Errorf("parameter %q enum value is below minimum byte length", parameter.Name)
		}
		if parameter.MaxBytes != nil && valueBytes > *parameter.MaxBytes {
			return fmt.Errorf("parameter %q enum value exceeds maximum byte length", parameter.Name)
		}
	}
	return nil
}

func validToolProfileID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	segments := strings.Split(value, ".")
	if len(segments) < 2 {
		return false
	}
	for _, segment := range segments {
		if len(segment) == 0 || !isASCIILower(segment[0]) {
			return false
		}
		for index := 1; index < len(segment); index++ {
			if !isASCIILower(segment[index]) && !isASCIIDigit(segment[index]) {
				return false
			}
		}
	}
	return true
}

func validToolProfileVersion(value string) bool {
	if len(value) == 0 || len(value) > 64 || !isASCIIAlphaNumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIIAlphaNumeric(character) && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func validToolMachineName(value string) bool {
	if len(value) == 0 || len(value) > 64 || !isASCIILower(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIILower(character) && !isASCIIDigit(character) && character != '_' {
			return false
		}
	}
	return true
}

func forbiddenToolMachineName(value string) bool {
	for _, fragment := range []string{"secret", "credential", "token", "password", "passwd"} {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	_, forbidden := map[string]struct{}{
		"api_key": {}, "apikey": {}, "private_key": {}, "access_key": {},
		"executable": {}, "exec": {}, "command": {}, "cmd": {}, "argv": {},
		"shell": {}, "cwd": {}, "env": {}, "environment": {}, "path": {},
		"bundle": {}, "limits": {}, "network": {}, "effects": {},
	}[value]
	return forbidden
}

func isASCIIAlphaNumeric(value byte) bool {
	return isASCIILower(value) || value >= 'A' && value <= 'Z' || isASCIIDigit(value)
}

func isASCIILower(value byte) bool {
	return value >= 'a' && value <= 'z'
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func cloneToolProfileDescriptor(descriptor ToolProfileDescriptor) ToolProfileDescriptor {
	copy := descriptor
	copy.Ports = make([]ToolPortDescriptor, len(descriptor.Ports))
	for index, port := range descriptor.Ports {
		copy.Ports[index] = port
		copy.Ports[index].AcceptedMediaTypes = append([]string(nil), port.AcceptedMediaTypes...)
		copy.Ports[index].Required = cloneToolBool(port.Required)
		copy.Ports[index].Role = cloneToolString(port.Role)
	}
	copy.Parameters = make([]ToolParameterSpec, len(descriptor.Parameters))
	for index, parameter := range descriptor.Parameters {
		copy.Parameters[index] = parameter
		copy.Parameters[index].Enum = append([]string(nil), parameter.Enum...)
		copy.Parameters[index].MinBytes = cloneToolInteger(parameter.MinBytes)
		copy.Parameters[index].MaxBytes = cloneToolInteger(parameter.MaxBytes)
		copy.Parameters[index].Minimum = cloneToolInteger(parameter.Minimum)
		copy.Parameters[index].Maximum = cloneToolInteger(parameter.Maximum)
	}
	return copy
}

func cloneToolBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneToolString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneToolInteger(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func toolDescriptorBool(value bool) *bool {
	return &value
}

func toolDescriptorString(value string) *string {
	return &value
}

func toolDescriptorInteger(value int64) *int64 {
	return &value
}
