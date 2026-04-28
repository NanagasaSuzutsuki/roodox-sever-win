package server

type CleanupHooks struct {
	OnMutation func()
	OnConflict func()
}

func (h CleanupHooks) TriggerMutation() {
	if h.OnMutation != nil {
		h.OnMutation()
	}
}

func (h CleanupHooks) TriggerConflict() {
	if h.OnConflict != nil {
		h.OnConflict()
	}
}
