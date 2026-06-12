package capability

import "testing"

func TestRegistryRegistersAndGetsCapability(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(Descriptor{
		Name:          "get_weather",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskLow,
		DefaultPolicy: DefaultPolicyAllow,
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	desc, ok := registry.Get("get_weather")
	if !ok {
		t.Fatal("expected get_weather capability")
	}
	if desc.Type != TypeNativeTool || desc.Source != SourceBuiltin || desc.Risk != RiskLow {
		t.Fatalf("unexpected descriptor: %#v", desc)
	}
}

func TestRegistryRejectsDuplicateCapabilityNames(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry(
		Descriptor{Name: "tool", Type: TypeNativeTool, Source: SourceBuiltin, Risk: RiskLow, DefaultPolicy: DefaultPolicyAllow},
		Descriptor{Name: "tool", Type: TypeNativeTool, Source: SourceBuiltin, Risk: RiskLow, DefaultPolicy: DefaultPolicyAllow},
	)
	if err == nil {
		t.Fatal("expected duplicate capability error")
	}
}

func TestDescriptorValidationRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	err := Descriptor{
		Name:          "",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskLow,
		DefaultPolicy: DefaultPolicyAllow,
	}.Validate()
	if err == nil {
		t.Fatal("expected missing name validation error")
	}

	err = Descriptor{
		Name:          "bad",
		Type:          CapabilityType("unknown"),
		Source:        SourceBuiltin,
		Risk:          RiskLow,
		DefaultPolicy: DefaultPolicyAllow,
	}.Validate()
	if err == nil {
		t.Fatal("expected invalid type validation error")
	}
}
