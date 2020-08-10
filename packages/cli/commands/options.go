package commands

// Options defines common options for commands
type Options struct {
	// Example of optional value
	//ProjectDirectory string `short:"d" long:"project-dir" description:"Path to project directory" optional:"yes" optional-value:"."`

	// Example of map with multiple default values
	// Users map[string]string `long:"Customers" description:"User e-mail map" default:"system:system@example.org" default:"admin:admin@example.org"`
}
