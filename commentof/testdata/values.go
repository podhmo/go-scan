package fixture

// toplevel comment values 0  :IGNORED:

// GroupedConsts is a group of consts @CG0
const (
	// ConstA is a const @CA0
	ConstA = "A" // line comment ConstA @CA1

	// ConstB is a const @CB0
	ConstB = "B"
	ConstC = "C" // line comment ConstC @CC1
)

// SingleConst is a single const @CS0
const SingleConst = "S" // line comment SingleConst @CS1

// GroupedVars is a group of vars @VG0
var (
	// VarA is a var @VA0
	VarA = "A" // line comment VarA @VA1

	// VarB is a var @VB0
	VarB = "B"
	VarC = "C" // line comment VarC @VC1
)

// SingleVar is a single var @VS0
var SingleVar = "S" // line comment SingleVar @VS1

// MultiVar is a multi var @VM0
var MultiVar1, MultiVar2 = "V1", "V2" // line comment MultiVar @VM1
