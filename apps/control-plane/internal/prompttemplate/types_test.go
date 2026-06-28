package prompttemplate

import (
	"testing"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name    string
		content string
		values  map[string]string
		want    string
		wantErr bool
	}{
		{
			name:    "simple replacement",
			content: "Hello {{name}}",
			values:  map[string]string{"name": "Alice"},
			want:    "Hello Alice",
			wantErr: false,
		},
		{
			name:    "multiple replacements",
			content: "Hello {{name}}, welcome to {{place}}",
			values:  map[string]string{"name": "Alice", "place": "Wonderland"},
			want:    "Hello Alice, welcome to Wonderland",
			wantErr: false,
		},
		{
			name:    "with spaces in token",
			content: "Hello {{ name }}",
			values:  map[string]string{"name": "Bob"},
			want:    "Hello Bob",
			wantErr: false,
		},
		{
			name:    "missing token",
			content: "Hello {{name}}",
			values:  map[string]string{},
			want:    "",
			wantErr: true,
		},
		{
			name:    "multiple missing tokens",
			content: "Hello {{name}}, welcome to {{place}}",
			values:  map[string]string{"name": "Alice"},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.content, tt.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Render() = %v, want %v", got, tt.want)
			}
		})
	}
}
