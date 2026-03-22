package app

// SetURLValidator replaces the webhook URL validator on AlertService.
// Intended for use in tests only (e.g. to allow httptest server URLs).
func (s *AlertService) SetURLValidator(fn func(string) error) {
	s.urlValidator = fn
}
