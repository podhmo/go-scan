package greeter

type Greeter interface {
	Greet() string
}

type Service struct {
	greeter Greeter
}

func New(g Greeter) *Service {
	return &Service{greeter: g}
}

func (s *Service) SayHello() {
	println(s.greeter.Greet())
}

func UnusedMethod() {}
