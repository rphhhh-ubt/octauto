package database

import (
	"testing"
	"testing/quick"
)

// **Feature: promo-tariff-discount, Property 6: Duplicate Activation Prevention**
// **Validates: Requirements 4.5**
// *For any* customer who already activated a promo tariff code, repeated activation
// of the same code should fail with "promo_already_used" error.

// ActivationState представляет состояние активаций промокода
type ActivationState struct {
	Activations map[int64]map[int64]bool // promoTariffID -> customerID -> activated
}

// NewActivationState создаёт новое состояние активаций
func NewActivationState() *ActivationState {
	return &ActivationState{
		Activations: make(map[int64]map[int64]bool),
	}
}

// IsUsedByCustomer проверяет, использовал ли пользователь промокод (модель)
func (s *ActivationState) IsUsedByCustomer(promoTariffID, customerID int64) bool {
	if customers, ok := s.Activations[promoTariffID]; ok {
		return customers[customerID]
	}
	return false
}

// RecordActivation записывает активацию (модель)
func (s *ActivationState) RecordActivation(promoTariffID, customerID int64) error {
	if s.IsUsedByCustomer(promoTariffID, customerID) {
		return ErrPromoTariffAlreadyUsed
	}
	if s.Activations[promoTariffID] == nil {
		s.Activations[promoTariffID] = make(map[int64]bool)
	}
	s.Activations[promoTariffID][customerID] = true
	return nil
}

// TryActivate пытается активировать промокод (модель бизнес-логики)
// Возвращает nil если активация успешна, ErrPromoTariffAlreadyUsed если уже активирован
func (s *ActivationState) TryActivate(promoTariffID, customerID int64) error {
	// Проверяем, не использовал ли уже пользователь этот промокод
	if s.IsUsedByCustomer(promoTariffID, customerID) {
		return ErrPromoTariffAlreadyUsed
	}
	// Записываем активацию
	return s.RecordActivation(promoTariffID, customerID)
}

func TestDuplicateActivationPreventionProperty(t *testing.T) {
	f := func(
		promoTariffIDRaw uint32,
		customerIDRaw uint32,
	) bool {
		// Ограничиваем ID разумными значениями
		promoTariffID := int64(promoTariffIDRaw%1000000) + 1
		customerID := int64(customerIDRaw%1000000) + 1

		state := NewActivationState()

		// Первая активация должна быть успешной
		err1 := state.TryActivate(promoTariffID, customerID)
		if err1 != nil {
			t.Logf("First activation should succeed, got error: %v", err1)
			return false
		}

		// PROPERTY: Повторная активация того же промокода тем же пользователем
		// должна вернуть ошибку ErrPromoTariffAlreadyUsed
		err2 := state.TryActivate(promoTariffID, customerID)
		if err2 != ErrPromoTariffAlreadyUsed {
			t.Logf("Second activation should return ErrPromoTariffAlreadyUsed, got: %v", err2)
			return false
		}

		// Третья попытка тоже должна вернуть ошибку
		err3 := state.TryActivate(promoTariffID, customerID)
		if err3 != ErrPromoTariffAlreadyUsed {
			t.Logf("Third activation should return ErrPromoTariffAlreadyUsed, got: %v", err3)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestDuplicateActivationDifferentCustomers проверяет, что разные пользователи
// могут активировать один и тот же промокод
func TestDuplicateActivationDifferentCustomers(t *testing.T) {
	f := func(
		promoTariffIDRaw uint32,
		customer1IDRaw uint32,
		customer2IDRaw uint32,
	) bool {
		promoTariffID := int64(promoTariffIDRaw%1000000) + 1
		customer1ID := int64(customer1IDRaw%1000000) + 1
		customer2ID := int64(customer2IDRaw%1000000) + 1

		// Если ID совпадают, пропускаем тест
		if customer1ID == customer2ID {
			return true
		}

		state := NewActivationState()

		// Первый пользователь активирует промокод
		err1 := state.TryActivate(promoTariffID, customer1ID)
		if err1 != nil {
			t.Logf("First customer activation should succeed, got error: %v", err1)
			return false
		}

		// PROPERTY: Другой пользователь может активировать тот же промокод
		err2 := state.TryActivate(promoTariffID, customer2ID)
		if err2 != nil {
			t.Logf("Second customer should be able to activate same promo, got error: %v", err2)
			return false
		}

		// Но первый пользователь не может активировать повторно
		err3 := state.TryActivate(promoTariffID, customer1ID)
		if err3 != ErrPromoTariffAlreadyUsed {
			t.Logf("First customer should not be able to re-activate, got: %v", err3)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestDuplicateActivationDifferentPromos проверяет, что один пользователь
// может активировать разные промокоды
func TestDuplicateActivationDifferentPromos(t *testing.T) {
	f := func(
		promo1IDRaw uint32,
		promo2IDRaw uint32,
		customerIDRaw uint32,
	) bool {
		promo1ID := int64(promo1IDRaw%1000000) + 1
		promo2ID := int64(promo2IDRaw%1000000) + 1
		customerID := int64(customerIDRaw%1000000) + 1

		// Если ID промокодов совпадают, пропускаем тест
		if promo1ID == promo2ID {
			return true
		}

		state := NewActivationState()

		// Пользователь активирует первый промокод
		err1 := state.TryActivate(promo1ID, customerID)
		if err1 != nil {
			t.Logf("First promo activation should succeed, got error: %v", err1)
			return false
		}

		// PROPERTY: Тот же пользователь может активировать другой промокод
		err2 := state.TryActivate(promo2ID, customerID)
		if err2 != nil {
			t.Logf("Customer should be able to activate different promo, got error: %v", err2)
			return false
		}

		// Но не может повторно активировать первый
		err3 := state.TryActivate(promo1ID, customerID)
		if err3 != ErrPromoTariffAlreadyUsed {
			t.Logf("Customer should not be able to re-activate first promo, got: %v", err3)
			return false
		}

		// И не может повторно активировать второй
		err4 := state.TryActivate(promo2ID, customerID)
		if err4 != ErrPromoTariffAlreadyUsed {
			t.Logf("Customer should not be able to re-activate second promo, got: %v", err4)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}

// TestIsUsedByCustomerConsistency проверяет консистентность IsUsedByCustomer
func TestIsUsedByCustomerConsistency(t *testing.T) {
	f := func(
		promoTariffIDRaw uint32,
		customerIDRaw uint32,
	) bool {
		promoTariffID := int64(promoTariffIDRaw%1000000) + 1
		customerID := int64(customerIDRaw%1000000) + 1

		state := NewActivationState()

		// До активации IsUsedByCustomer должен возвращать false
		if state.IsUsedByCustomer(promoTariffID, customerID) {
			t.Logf("IsUsedByCustomer should return false before activation")
			return false
		}

		// Активируем
		_ = state.TryActivate(promoTariffID, customerID)

		// PROPERTY: После активации IsUsedByCustomer должен возвращать true
		if !state.IsUsedByCustomer(promoTariffID, customerID) {
			t.Logf("IsUsedByCustomer should return true after activation")
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Error(err)
	}
}
