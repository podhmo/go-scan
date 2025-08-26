package repro

type Service struct{}

func (s *Service) unexportedMethod() {}

func (s *Service) ExportedMethod() {
	s.unexportedMethod()
}

// This function should not be an orphan, as it's an exported entry point.
func ExportedUnusedFunc() {}
