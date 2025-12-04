package remnawave

import (
	"testing"
	"testing/quick"
)

// **Feature: tariff-system, Property 1: Disabled Limit Protection**
// *For any* user with disabled limit (nil), ResolveDeviceLimit SHALL return nil.
func TestResolveDeviceLimit_DisabledLimit(t *testing.T) {
	f := func(tariffLimit int) bool {
		result := ResolveDeviceLimit(nil, tariffLimit)
		return result == nil
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 1 (disabled limit) failed: %v", err)
	}
}

// **Feature: tariff-system, Property 2: Personal Limit Replacement**
// *For any* user with personal limit, ResolveDeviceLimit SHALL return tariffLimit.
func TestResolveDeviceLimit_PersonalLimit(t *testing.T) {
	f := func(currentLimit int, tariffLimit int) bool {
		result := ResolveDeviceLimit(&currentLimit, tariffLimit)
		return result != nil && *result == tariffLimit
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 2 (personal limit) failed: %v", err)
	}
}

// **Feature: tariff-system: Concrete test cases**
func TestResolveDeviceLimit_Scenarios(t *testing.T) {
	tests := []struct {
		name         string
		currentLimit *int
		tariffLimit  int
		expected     *int
	}{
		// Лимит отключен → не трогаем
		{"disabled → 3", nil, 3, nil},
		{"disabled → 6", nil, 6, nil},

		// Персональный → заменить на новый тариф
		{"3 → 6 (upgrade)", intPtr(3), 6, intPtr(6)},
		{"6 → 3 (downgrade)", intPtr(6), 3, intPtr(3)},
		{"3 → 1 (downgrade)", intPtr(3), 1, intPtr(1)},
		{"1 → 1 (same)", intPtr(1), 1, intPtr(1)},
		{"5 → 6", intPtr(5), 6, intPtr(6)},
		{"5 → 3", intPtr(5), 3, intPtr(3)},
		{"10 → 6", intPtr(10), 6, intPtr(6)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveDeviceLimit(tt.currentLimit, tt.tariffLimit)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", *result)
				}
			} else {
				if result == nil || *result != *tt.expected {
					t.Errorf("expected %d, got %v", *tt.expected, result)
				}
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}
