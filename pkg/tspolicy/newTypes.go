package tspolicy

var POLICY_WEIGHTS = map[string]int{
	"DEFAULT": 0.0,
	"PREFER":  8.0,
	"AVOID":   -8.0,
	"SHALL":   1000.0,
	"FORBID":  -1000.0,
}
