package fbauth

import (
	"context"
	"fmt"
	"log"
)

func Main() {
	ctx := context.Background()
	projects, err := GetProjects(ctx)
	if err != nil {
		log.Fatalf("Unable to get projects: %v", err)
	}
	for _, project := range projects {
		fmt.Printf("Project ID: %s, Name: %s, State: %s\n", project.ProjectId, project.DisplayName, project.State)
	}
}
