package workflows

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     SubmitRequest
		wantErr bool
	}{
		{"valid", SubmitRequest{Name: "build", Steps: []StepRequest{{Key: "compile", Type: "echo"}, {Key: "test", Type: "echo", DependsOn: []string{"compile"}}}}, false},
		{"cycle", SubmitRequest{Name: "bad", Steps: []StepRequest{{Key: "a", Type: "echo", DependsOn: []string{"b"}}, {Key: "b", Type: "echo", DependsOn: []string{"a"}}}}, true},
		{"missing", SubmitRequest{Name: "bad", Steps: []StepRequest{{Key: "a", Type: "echo", DependsOn: []string{"x"}}}}, true},
		{"duplicate", SubmitRequest{Name: "bad", Steps: []StepRequest{{Key: "a", Type: "echo"}, {Key: "a", Type: "echo"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
