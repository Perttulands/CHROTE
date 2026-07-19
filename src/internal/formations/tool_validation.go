package formations

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

const (
	maxToolParameters          = 16
	maxToolParameterObjectSize = 4096
)

func validateToolNodeAgainstDescriptor(tool ToolNode, descriptor ToolProfileDescriptor) error {
	if tool.ID == "" {
		return fmt.Errorf("Tool id is empty")
	}
	if tool.ProfileID != descriptor.ProfileID || tool.ProfileVersion != descriptor.ProfileVersion {
		return fmt.Errorf("Tool %q profile tuple %q@%q does not match descriptor %q@%q", tool.ID, tool.ProfileID, tool.ProfileVersion, descriptor.ProfileID, descriptor.ProfileVersion)
	}
	if err := validateToolPortsAgainstDescriptor(tool, descriptor); err != nil {
		return err
	}
	if err := validateToolParametersAgainstDescriptor(tool, descriptor.Parameters); err != nil {
		return err
	}
	return nil
}

func validateToolPortsAgainstDescriptor(tool ToolNode, descriptor ToolProfileDescriptor) error {
	inputs := make([]ToolPortDescriptor, 0, len(descriptor.Ports))
	outputs := make([]ToolPortDescriptor, 0, len(descriptor.Ports))
	for _, port := range descriptor.Ports {
		switch port.Direction {
		case FormationPortInput:
			inputs = append(inputs, port)
		case FormationPortOutput:
			outputs = append(outputs, port)
		}
	}
	if len(tool.Inputs) != len(inputs) || len(tool.Outputs) != len(outputs) {
		return fmt.Errorf("Tool %q ports do not match descriptor", tool.ID)
	}

	portIDs := make(map[string]struct{}, len(tool.Inputs)+len(tool.Outputs))
	for index, port := range tool.Inputs {
		if err := validateToolPortBinding(tool.ID, port, inputs[index], portIDs); err != nil {
			return err
		}
	}
	for index, port := range tool.Outputs {
		if err := validateToolPortBinding(tool.ID, port, outputs[index], portIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateToolPortBinding(toolID string, port ToolPort, descriptor ToolPortDescriptor, portIDs map[string]struct{}) error {
	if port.ID == "" {
		return fmt.Errorf("Tool %q has an empty port id", toolID)
	}
	if _, exists := portIDs[port.ID]; exists {
		return fmt.Errorf("Tool %q has duplicate port id %q", toolID, port.ID)
	}
	portIDs[port.ID] = struct{}{}

	if port.Name != descriptor.Name ||
		port.Direction != descriptor.Direction ||
		port.Kind != descriptor.Kind ||
		!equalToolStrings(port.AcceptedMediaTypes, descriptor.AcceptedMediaTypes) ||
		!equalToolBool(port.Required, descriptor.Required) ||
		!equalToolString(port.Role, descriptor.Role) {
		return fmt.Errorf("Tool %q port %q does not match descriptor binding %q", toolID, port.ID, descriptor.Name)
	}
	return nil
}

func validateToolParametersAgainstDescriptor(tool ToolNode, parameters []ToolParameterSpec) error {
	if len(parameters) > maxToolParameters || len(tool.Params) > maxToolParameters {
		return fmt.Errorf("Tool %q exceeds %d parameters", tool.ID, maxToolParameters)
	}

	byName := make(map[string]ToolParameterSpec, len(parameters))
	for _, parameter := range parameters {
		byName[parameter.Name] = parameter
		value, present := tool.Params[parameter.Name]
		if !present {
			if parameter.Required {
				return fmt.Errorf("Tool %q is missing required parameter %q", tool.ID, parameter.Name)
			}
			continue
		}
		if err := validateToolParameterValue(parameter, value); err != nil {
			return fmt.Errorf("Tool %q parameter %q: %w", tool.ID, parameter.Name, err)
		}
	}
	unknownNames := make([]string, 0)
	for name := range tool.Params {
		if _, declared := byName[name]; !declared {
			unknownNames = append(unknownNames, name)
		}
	}
	sort.Strings(unknownNames)
	if len(unknownNames) > 0 {
		return fmt.Errorf("Tool %q has unknown parameter %q", tool.ID, unknownNames[0])
	}

	canonical, err := canonicalToolParameterObject(tool.Params)
	if err != nil {
		return fmt.Errorf("Tool %q parameters: %w", tool.ID, err)
	}
	if len(canonical) > maxToolParameterObjectSize {
		return fmt.Errorf("Tool %q parameter object exceeds %d canonical bytes", tool.ID, maxToolParameterObjectSize)
	}
	return nil
}

func validateToolParameterValue(parameter ToolParameterSpec, value any) error {
	switch parameter.Type {
	case "string":
		stringValue, ok := value.(string)
		if !ok || !validToolString(stringValue) {
			return fmt.Errorf("must be a valid UTF-8 NUL-free string")
		}
		length := int64(len(stringValue))
		if parameter.MinBytes != nil && length < *parameter.MinBytes {
			return fmt.Errorf("is shorter than %d bytes", *parameter.MinBytes)
		}
		if parameter.MaxBytes != nil && length > *parameter.MaxBytes {
			return fmt.Errorf("is longer than %d bytes", *parameter.MaxBytes)
		}
		if len(parameter.Enum) > 0 && !containsToolString(parameter.Enum, stringValue) {
			return fmt.Errorf("is outside the allowed enum")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be a boolean")
		}
	case "integer":
		integerValue, ok := value.(int64)
		if !ok {
			return fmt.Errorf("must be an integer")
		}
		if integerValue < -maxToolParameterInteger || integerValue > maxToolParameterInteger {
			return fmt.Errorf("is outside JSON-safe integer bounds")
		}
		if parameter.Minimum != nil && integerValue < *parameter.Minimum {
			return fmt.Errorf("is below %d", *parameter.Minimum)
		}
		if parameter.Maximum != nil && integerValue > *parameter.Maximum {
			return fmt.Errorf("is above %d", *parameter.Maximum)
		}
	default:
		return fmt.Errorf("has unsupported descriptor type %q", parameter.Type)
	}
	return nil
}

func canonicalToolParameterObject(parameters map[string]any) ([]byte, error) {
	names := make([]string, 0, len(parameters))
	for name := range parameters {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonical bytes.Buffer
	canonical.WriteByte('{')
	for index, name := range names {
		if index > 0 {
			canonical.WriteByte(',')
		}
		writeRuntimeCanonicalJSONString(&canonical, name)
		canonical.WriteByte(':')
		switch value := parameters[name].(type) {
		case string:
			if !validToolString(value) {
				return nil, fmt.Errorf("parameter %q is not a valid UTF-8 NUL-free string", name)
			}
			writeRuntimeCanonicalJSONString(&canonical, value)
		case bool:
			canonical.WriteString(strconv.FormatBool(value))
		case int64:
			canonical.WriteString(strconv.FormatInt(value, 10))
		default:
			return nil, fmt.Errorf("parameter %q is not a scalar", name)
		}
	}
	canonical.WriteByte('}')
	return canonical.Bytes(), nil
}

func equalToolStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalToolBool(left, right *bool) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func equalToolString(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func containsToolString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func toolEndpointAllowsDirection(raw []byte, nodeID, portID, direction string) bool {
	tools, err := parseToolNodes(raw)
	if err != nil {
		return false
	}
	for _, tool := range tools {
		if tool.ID != nodeID {
			continue
		}
		ports := tool.Inputs
		if direction == FormationPortOutput {
			ports = tool.Outputs
		} else if direction != FormationPortInput {
			return false
		}
		for _, port := range ports {
			if port.ID == portID {
				return true
			}
		}
	}
	return false
}

func endpointPayloadContract(board *BoardDocument, endpoint, direction string) (string, []string, bool, bool) {
	nodeID, portID, ok := splitEndpoint(endpoint)
	if !ok {
		return "", nil, false, false
	}
	for _, tool := range board.Tools {
		if tool.ID != nodeID {
			continue
		}
		ports := tool.Inputs
		if direction == FormationPortOutput {
			ports = tool.Outputs
		} else if direction != FormationPortInput {
			return "", nil, true, false
		}
		for _, port := range ports {
			if port.ID == portID {
				return port.Kind, port.AcceptedMediaTypes, true, true
			}
		}
		return "", nil, true, false
	}
	for _, mission := range board.Missions {
		if mission.ID == nodeID {
			if direction == FormationPortOutput && portID == "out" {
				return "work", []string{"text/markdown"}, false, true
			}
			return "", nil, false, false
		}
	}
	for _, formation := range board.Formations {
		if formation.ID != nodeID {
			continue
		}
		ports := formation.Inputs
		if direction == FormationPortOutput {
			ports = formation.Outputs
		} else if direction != FormationPortInput {
			return "", nil, false, false
		}
		for _, port := range ports {
			if port.ID == portID {
				return "work", allToolWorkMediaTypes(), false, true
			}
		}
		return "", nil, false, false
	}
	for _, gate := range board.Gates {
		if gate.ID != nodeID {
			continue
		}
		if direction == FormationPortInput && portID == "in" || direction == FormationPortOutput && portID == "pass" {
			return "work", allToolWorkMediaTypes(), false, true
		}
		if direction == FormationPortOutput && portID == "fail" {
			return "gate_feedback", nil, false, true
		}
		return "", nil, false, false
	}
	return "", nil, false, false
}

func toolConnectionCompatibilityFinding(board *BoardDocument, connection BoardConnection) (BoardFinding, bool) {
	if board == nil {
		return BoardFinding{}, false
	}

	producerKind, producerMedia, producerIsTool, producerOK := endpointPayloadContract(board, connection.From, FormationPortOutput)
	consumerKind, consumerMedia, consumerIsTool, consumerOK := endpointPayloadContract(board, connection.To, FormationPortInput)
	toolProducesToJudge := producerIsTool && producerOK && isGateJudgeEndpoint(board, connection.To)
	judgeProducesToTool := consumerIsTool && consumerOK && isGateJudgeEndpoint(board, connection.From)
	if toolProducesToJudge || judgeProducesToTool {
		return BoardFinding{
			Code:    FindingInvalidJudgeRelationship,
			NodeID:  connection.ID,
			Message: fmt.Sprintf("connection %q cannot route a Tool endpoint through Gate judge endpoint", connection.ID),
		}, true
	}
	if !(producerIsTool || consumerIsTool) || !producerOK || !consumerOK {
		return BoardFinding{}, false
	}
	if producerKind != consumerKind {
		return BoardFinding{
			Code:    FindingIncompatiblePayloadKind,
			NodeID:  connection.ID,
			Message: fmt.Sprintf("connection %q routes payload kind %q to input %q, which accepts %q", connection.ID, producerKind, connection.To, consumerKind),
		}, true
	}
	if producerKind == "work" && !toolMediaSubset(producerMedia, consumerMedia) {
		return BoardFinding{
			Code:    FindingIncompatibleMedia,
			NodeID:  connection.ID,
			Message: fmt.Sprintf("connection %q routes producer media %v to input %q, which accepts %v", connection.ID, producerMedia, connection.To, consumerMedia),
		}, true
	}
	return BoardFinding{}, false
}

func isGateJudgeEndpoint(board *BoardDocument, endpoint string) bool {
	nodeID, portID, ok := splitEndpoint(endpoint)
	if !ok || portID != "judge" {
		return false
	}
	for _, gate := range board.Gates {
		if gate.ID == nodeID {
			return true
		}
	}
	return false
}

func allToolWorkMediaTypes() []string {
	return []string{"text/plain", "text/markdown", "application/json"}
}

func toolMediaSubset(producer, consumer []string) bool {
	for _, mediaType := range producer {
		if !containsToolString(consumer, mediaType) {
			return false
		}
	}
	return true
}
