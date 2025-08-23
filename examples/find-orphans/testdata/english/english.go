package english

type Greeter struct{}

func (g *Greeter) Greet() string {
	return "Hello"
}

func (g *Greeter) GreetFormal() string {
	return "Greetings"
}
