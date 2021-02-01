package commands

type projectDirCommand struct {
	ProjectDir  string `short:"d" long:"directory"  required:"false" description:"Project directory"`
}

// ProjectBaseCommand defines parameters for show project command
type projectBaseCommand struct {
	projectDirCommand
	ProjectName string `short:"n" long:"name"  required:"false" description:"Project name"`
}


