package recursion

// DirectRecursion is a simple function that calls itself.
func DirectRecursion() {
	DirectRecursion()
}

// MutualRecursionA calls MutualRecursionB.
func MutualRecursionA() {
	MutualRecursionB()
}

// MutualRecursionB calls MutualRecursionA.
func MutualRecursionB() {
	MutualRecursionA()
}