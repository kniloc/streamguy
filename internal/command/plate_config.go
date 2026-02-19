package command

type plateConfig struct {
	Color  string
	Format string
}

var plateConfigs = map[string]plateConfig{
	"AL": {Color: "#000", Format: "AB123CD"},
	"AK": {Color: "#032a6b", Format: "A12 C3D"},
	"AZ": {Color: "#08362c", Format: "AB1 23C"},
	"AR": {Color: "#000", Format: "12A BCD"},
	"CA": {Color: "#0d2251", Format: "A123BCD"},
	"CO": {Color: "#061010", Format: "12A BC3"},
	"CT": {Color: "#062146", Format: "12 345AB"},
	"DE": {Color: "#CCB45F", Format: "ABC123"},
	"DC": {Color: "#021D8C", Format: "12 34AB"},
	"FL": {Color: "#005E48", Format: "AB123CD"},
	"GA": {Color: "#000", Format: "1ABC234"},
	"HI": {Color: "#000", Format: "123ABC"},
	"IA": {Color: "#000", Format: "1234AB"},
	"ID": {Color: "#000", Format: "A1 BCD23"},
	"IL": {Color: "#FFF", Format: "A12BC3"},
	"IN": {Color: "#FFF", Format: "12ABC3"},
	"KS": {Color: "#FFF", Format: "ABC1234"},
	"KY": {Color: "#1E2767", Format: "1A23BC"},
}
