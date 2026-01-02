/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s3clientimpl

import (
	"testing"
)

func TestConstructEndpointFromURL(t *testing.T) {
	tests := []struct {
		name     string
		inputURL string
		wantEP   string
		wantPort string
		wantSSL  bool
		wantErr  bool
	}{
		{
			name:     "https without port",
			inputURL: "https://example.com",
			wantEP:   "example.com",
			wantPort: "",
			wantSSL:  true,
			wantErr:  false,
		},
		{
			name:     "https with default port 443",
			inputURL: "https://example.com:443",
			wantEP:   "example.com",
			wantPort: "443",
			wantSSL:  true,
			wantErr:  false,
		},
		{
			name:     "http without port",
			inputURL: "http://example.com",
			wantEP:   "example.com",
			wantPort: "",
			wantSSL:  false,
			wantErr:  false,
		},
		{
			name:     "http with default port 80",
			inputURL: "http://example.com:80",
			wantEP:   "example.com",
			wantPort: "80",
			wantSSL:  false,
			wantErr:  false,
		},
		{
			name:     "https with non standard port",
			inputURL: "https://example.com:8443",
			wantEP:   "example.com:8443",
			wantPort: "8443",
			wantSSL:  true,
			wantErr:  false,
		},
		{
			name:     "http with non standard port",
			inputURL: "http://example.com:8080",
			wantEP:   "example.com:8080",
			wantPort: "8080",
			wantSSL:  false,
			wantErr:  false,
		},
		{
			name:     "invalid url",
			inputURL: "://bad-url",
			wantEP:   "",
			wantPort: "",
			wantSSL:  false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, port, scheme, err := ConstructEndpointFromURL(tt.inputURL)
			ssl := scheme == "https"
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if endpoint != tt.wantEP {
				t.Errorf("endpoint = %q, want %q", endpoint, tt.wantEP)
			}

			if port != tt.wantPort {
				t.Errorf("port = %q, want %q", port, tt.wantPort)
			}

			if ssl != tt.wantSSL {
				t.Errorf("ssl = %v, want %v", ssl, tt.wantSSL)
			}
		})
	}
}
