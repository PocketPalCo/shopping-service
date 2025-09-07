package handlers

import (
	"fmt"
	"log/slog"
	"sync"
)

// StateManager manages temporary user states for workflows
type StateManager struct {
	states      map[string]string
	statesMutex sync.RWMutex
	logger      *slog.Logger
}

// NewStateManager creates a new state manager
func NewStateManager(logger *slog.Logger) *StateManager {
	return &StateManager{
		states: make(map[string]string),
		logger: logger,
	}
}

// SetUserState stores temporary user state
func (sm *StateManager) SetUserState(telegramID int64, state, value string) {
	sm.statesMutex.Lock()
	defer sm.statesMutex.Unlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	sm.states[key] = value

	sm.logger.Info("Setting user state", "telegram_id", telegramID, "state", state, "value", value)
}

// GetUserState retrieves temporary user state
func (sm *StateManager) GetUserState(telegramID int64, state string) (string, bool) {
	sm.statesMutex.RLock()
	defer sm.statesMutex.RUnlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	value, exists := sm.states[key]
	return value, exists
}

// ClearUserState removes temporary user state
func (sm *StateManager) ClearUserState(telegramID int64, state string) {
	sm.statesMutex.Lock()
	defer sm.statesMutex.Unlock()

	key := fmt.Sprintf("%d:%s", telegramID, state)
	delete(sm.states, key)

	sm.logger.Info("Clearing user state", "telegram_id", telegramID, "state", state)
}

// ClearAllUserStates removes all states for a specific user
func (sm *StateManager) ClearAllUserStates(telegramID int64) {
	sm.statesMutex.Lock()
	defer sm.statesMutex.Unlock()

	prefix := fmt.Sprintf("%d:", telegramID)
	for key := range sm.states {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(sm.states, key)
		}
	}

	sm.logger.Info("Clearing all user states", "telegram_id", telegramID)
}

// GetAllStates returns all current states (for debugging)
func (sm *StateManager) GetAllStates() map[string]string {
	sm.statesMutex.RLock()
	defer sm.statesMutex.RUnlock()

	// Return a copy to avoid race conditions
	states := make(map[string]string)
	for k, v := range sm.states {
		states[k] = v
	}
	return states
}
