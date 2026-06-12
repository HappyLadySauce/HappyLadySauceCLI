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

	// missing name — name 缺失
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

	// unknown type string (matches TypeUnknown constant) — 类型为 unknown
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

	// empty source — source 为空
	err = Descriptor{
		Name:          "missing_source",
		Type:          TypeNativeTool,
		Source:        "",
		Risk:          RiskLow,
		DefaultPolicy: DefaultPolicyAllow,
	}.Validate()
	if err == nil {
		t.Fatal("expected missing source validation error")
	}

	// invalid risk level — 无效风险等级
	err = Descriptor{
		Name:          "bad_risk",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskLevel("extreme"),
		DefaultPolicy: DefaultPolicyAllow,
	}.Validate()
	if err == nil {
		t.Fatal("expected invalid risk level validation error")
	}

	// invalid default policy — 无效默认策略
	err = Descriptor{
		Name:          "bad_policy",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskLow,
		DefaultPolicy: DefaultPolicy("auto"),
	}.Validate()
	if err == nil {
		t.Fatal("expected invalid default policy validation error")
	}
}

func TestDescriptorValidationJoinsMultipleErrors(t *testing.T) {
	t.Parallel()

	// Multiple invalid fields: whitespace-only name, TypeUnknown, empty source, invalid risk.
	// 多个无效字段：纯空白名称、TypeUnknown、空 source、无效风险等级。
	descriptor := Descriptor{
		Name:          "   ",
		Type:          TypeUnknown,
		Source:        "",
		Risk:          RiskLevel("critical"),
		DefaultPolicy: DefaultPolicyAllow,
	}
	err := descriptor.Validate()
	if err == nil {
		t.Fatal("expected validation error for multiple invalid fields")
	}
	if len(err.Error()) == 0 {
		t.Fatal("expected non-empty error message")
	}
}

func TestDescriptorValidationRequiresNetworkResourcesAllowlist(t *testing.T) {
	t.Parallel()

	err := Descriptor{
		Name:          "fetch",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskMedium,
		DefaultPolicy: DefaultPolicyReview,
		Scopes:        []string{"network:api"},
	}.Validate()
	if err == nil {
		t.Fatal("expected network scope without Resources to fail validation")
	}
}

func TestDescriptorValidationRequiresNetworkScopeForHTTPResources(t *testing.T) {
	t.Parallel()

	err := Descriptor{
		Name:          "fetch",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskMedium,
		DefaultPolicy: DefaultPolicyReview,
		Resources:     []string{"https://example.com/api"},
	}.Validate()
	if err == nil {
		t.Fatal("expected http Resources without network scope to fail validation")
	}
}

func TestDescriptorValidationRejectsEmptyNetworkScopeSuffix(t *testing.T) {
	t.Parallel()

	err := Descriptor{
		Name:          "fetch",
		Type:          TypeNativeTool,
		Source:        SourceBuiltin,
		Risk:          RiskMedium,
		DefaultPolicy: DefaultPolicyReview,
		Scopes:        []string{"network:"},
		Resources:     []string{"https://example.com/api"},
	}.Validate()
	if err == nil {
		t.Fatal("expected empty network: scope suffix to fail validation")
	}
}
