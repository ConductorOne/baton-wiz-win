package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *WizWin
		wantErr bool
	}{
		{
			name: "valid config",
			config: &WizWin{
				WizApiUrl:       "https://api.wiz.io/graphql",
				WizClientId:     "test-client-id",
				WizClientSecret: "test-client-secret",
				WizAuthEndpoint: "https://auth.wiz.io/oauth/token",
			},
			wantErr: false,
		},
		{
			name: "invalid config - missing required fields",
			config: &WizWin{
				WizApiUrl: "https://api.wiz.io/graphql",
				// Missing other required fields
			},
			wantErr: true,
		},
		{
			name:    "invalid config - all fields missing",
			config:  &WizWin{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := field.Validate(Config, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
