package cli

import (
	"strings"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
)

func TestSelectSearchModel(t *testing.T) {
	m1 := config.ModelConfig{Name: "m1"}
	m2default := config.ModelConfig{Name: "m2", DefaultForSearch: true}

	cases := []struct {
		name    string
		cfg     config.EmbeddingConfig
		request string
		want    string
		errSubs string
	}{
		{
			name:    "single model without default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1}},
			request: "",
			want:    "m1",
		},
		{
			name:    "single model with default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, default picked when request empty",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, explicit name wins over default",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "m1",
			want:    "m1",
		},
		{
			name:    "two models, missing name returns error mentioning it",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "ghost",
			errSubs: "ghost",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectSearchModel(tc.cfg, tc.request)
			if tc.errSubs != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.errSubs)
				}
				if !strings.Contains(err.Error(), tc.errSubs) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tc.want {
				t.Fatalf("want model %q, got %q", tc.want, got.Name)
			}
		})
	}
}
