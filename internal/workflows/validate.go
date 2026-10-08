package workflows

import (
	"encoding/json"
	"errors"
	"fmt"
)

func Validate(r SubmitRequest) error {
	if r.Name == "" {
		return errors.New("workflow name is required")
	}
	if len(r.Steps) == 0 {
		return errors.New("at least one step is required")
	}
	steps := make(map[string]StepRequest, len(r.Steps))
	for _, s := range r.Steps {
		if s.Key == "" || s.Type == "" {
			return errors.New("each step needs key and type")
		}
		if _, ok := steps[s.Key]; ok {
			return fmt.Errorf("duplicate step key %q", s.Key)
		}
		steps[s.Key] = s
		if s.Compensation != nil && (s.Compensation.Type == "" || !json.Valid(s.Compensation.Payload)) {
			return fmt.Errorf("step %q has invalid compensation", s.Key)
		}
		switch s.Condition {
		case "", "success()", "failure()", "always()":
		default:
			return fmt.Errorf("step %q has unsupported condition %q", s.Key, s.Condition)
		}
	}
	for _, s := range r.Steps {
		if s.FanOutFrom != "" {
			if _, ok := steps[s.FanOutFrom]; !ok {
				return fmt.Errorf("step %q has unknown fan_out_from %q", s.Key, s.FanOutFrom)
			}
		}
		for _, d := range s.DependsOn {
			if d == s.Key {
				return fmt.Errorf("step %q depends on itself", s.Key)
			}
			if _, ok := steps[d]; !ok {
				return fmt.Errorf("step %q has unknown dependency %q", s.Key, d)
			}
		}
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(k string) error {
		if state[k] == 1 {
			return fmt.Errorf("workflow contains a cycle at %q", k)
		}
		if state[k] == 2 {
			return nil
		}
		state[k] = 1
		for _, d := range steps[k].DependsOn {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[k] = 2
		return nil
	}
	for k := range steps {
		if err := visit(k); err != nil {
			return err
		}
	}
	return nil
}
