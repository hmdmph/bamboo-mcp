package context

import (
	"path/filepath"
	"testing"
)

// newTestContext builds a PlanContext backed by a temp file with the given plan types.
func newTestContext(t *testing.T, planTypes []PlanType) *PlanContext {
	t.Helper()
	return &PlanContext{
		filePath: filepath.Join(t.TempDir(), "context.yaml"),
		config: &ContextConfig{
			DefaultProject: "EXAMPLE",
			PlanTypes:      planTypes,
		},
	}
}

func TestParseEnvironmentName(t *testing.T) {
	planTypes := []PlanType{
		{
			Type:      "app",
			EnvFormat: "<env>_<action>",
			Actions:   []ActionDetail{{Name: "deploy", Description: "Deploy the app"}},
		},
		{
			Type:      "infra",
			EnvFormat: "<env>_<module>_<ref>_<action>",
			Modules:   []string{"network", "database", "load_balancer"},
			ModuleDescriptions: map[string]string{
				"load_balancer": "Load balancers",
			},
			Actions: []ActionDetail{{Name: "deploy", Description: "Apply changes"}},
		},
		{
			Type:      "nodes",
			EnvFormat: "<env>_nodegroup_<action>",
		},
		{
			Type:             "registry",
			FixedDeployments: []string{"create_repos_prod", "promote_to_prod"},
		},
	}

	tests := []struct {
		name     string
		envName  string
		planType string
		want     map[string]string
	}{
		{
			name:     "simple env and action",
			envName:  "dev_deploy",
			planType: "app",
			want: map[string]string{
				"environment":        "dev",
				"action":             "deploy",
				"action_description": "Deploy the app",
			},
		},
		{
			name:     "multi-word action absorbed by final placeholder",
			envName:  "prod_restart_all_in_ns",
			planType: "app",
			want:     map[string]string{"environment": "prod", "action": "restart_all_in_ns"},
		},
		{
			name:     "single-word module",
			envName:  "staging_network_checkout_deploy",
			planType: "infra",
			want: map[string]string{
				"environment": "staging",
				"module":      "network",
				"ref":         "checkout",
				"action":      "deploy",
			},
		},
		{
			name:     "module containing an underscore is matched greedily",
			envName:  "prod_load_balancer_web_deploy",
			planType: "infra",
			want: map[string]string{
				"environment":        "prod",
				"module":             "load_balancer",
				"ref":                "web",
				"action":             "deploy",
				"module_description": "Load balancers",
			},
		},
		{
			name:     "unknown module falls back to remaining",
			envName:  "dev_mystery_thing_deploy",
			planType: "infra",
			want:     map[string]string{"environment": "dev", "remaining": "mystery_thing_deploy"},
		},
		{
			name:     "literal segment in format is recorded as marker",
			envName:  "prod_nodegroup_drain_apps",
			planType: "nodes",
			want:     map[string]string{"environment": "prod", "marker": "nodegroup", "action": "drain_apps"},
		},
		{
			name:     "fixed deployment names are matched exactly",
			envName:  "promote_to_prod",
			planType: "registry",
			want:     map[string]string{"deployment_name": "promote_to_prod", "matched_fixed_deployment": "promote_to_prod"},
		},
		{
			name:     "unknown plan type splits off leading environment",
			envName:  "dev_something_else",
			planType: "nope",
			want:     map[string]string{"environment": "dev", "remaining": "something_else"},
		},
	}

	pc := newTestContext(t, planTypes)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pc.ParseEnvironmentName(tt.envName, tt.planType)
			if got["raw"] != tt.envName {
				t.Errorf("raw = %q, want %q", got["raw"], tt.envName)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("%s = %q, want %q (full result: %v)", k, got[k], want, got)
				}
			}
		})
	}
}

func TestResolvePlanKey(t *testing.T) {
	pc := newTestContext(t, []PlanType{
		{Type: "app", Suffix: "App"},
		{Type: "secrets", Suffix: "Secrets Manager"},
		{Type: "registry", PlanKeyMatch: "EXAMPLE-REGISTRY"},
	})

	tests := []struct {
		name     string
		ref      string
		planType string
		wantKey  string
		wantErr  bool
	}{
		{name: "suffix appended and uppercased", ref: "checkout", planType: "app", wantKey: "EXAMPLE-CHECKOUTAPP"},
		{name: "spaces stripped from suffix", ref: "checkout", planType: "secrets", wantKey: "EXAMPLE-CHECKOUTSECRETSMANAGER"},
		{name: "plan_key_match short-circuits", ref: "anything", planType: "registry", wantKey: "EXAMPLE-REGISTRY"},
		{name: "no plan type does a direct lookup", ref: "checkout", planType: "", wantKey: "EXAMPLE-CHECKOUT"},
		{name: "unknown plan type errors", ref: "checkout", planType: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, _, err := pc.ResolvePlanKey(tt.ref, tt.planType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got key %q", key)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestDefaultConfigIsGeneric(t *testing.T) {
	cfg := defaultConfig()
	if cfg.DefaultProject == "" {
		t.Error("default project must not be empty")
	}
	if len(cfg.PlanTypes) == 0 {
		t.Error("default config should ship at least one example plan type")
	}
	for _, pt := range cfg.PlanTypes {
		if pt.Type == "" {
			t.Errorf("plan type with empty type name: %+v", pt)
		}
	}
}
