package mylib

// Messenger is an interface for getting messages.
type Messenger interface {
	GetMessage() string
}

// English a concrete implementation of Messenger.
type English struct{}

func (e *English) GetMessage() string {
	return "Hello"
}

// Japanese is another concrete implementation of Messenger.
type Japanese struct{}

func (j *Japanese) GetMessage() string {
	return "こんにちは"
}

// GetMessengers returns a slice of all available messengers.
func GetMessengers() []Messenger {
	return []Messenger{&English{}, &Japanese{}}
}
