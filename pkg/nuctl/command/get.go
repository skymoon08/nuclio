/*
Copyright 2017 The Nuclio Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package command

import (
	"fmt"
	"io"
	"strconv"

	"github.com/nuclio/nuclio/pkg/common"
	"github.com/nuclio/nuclio/pkg/errors"
	"github.com/nuclio/nuclio/pkg/platform"
	"github.com/nuclio/nuclio/pkg/renderer"

	"github.com/spf13/cobra"
)

const (
	outputFormatText = "text"
	outputFormatWide = "wide"
)

type getCommandeer struct {
	cmd            *cobra.Command
	rootCommandeer *RootCommandeer
}

func newGetCommandeer(rootCommandeer *RootCommandeer) *getCommandeer {
	commandeer := &getCommandeer{
		rootCommandeer: rootCommandeer,
	}

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Display resource information",
	}

	getFunctionCommand := newGetFunctionCommandeer(commandeer).cmd
	getProjectCommand := newGetProjectCommandeer(commandeer).cmd

	cmd.AddCommand(getFunctionCommand, getProjectCommand)

	commandeer.cmd = cmd

	return commandeer
}

type getFunctionCommandeer struct {
	*getCommandeer
	getFunctionsOptions platform.GetFunctionsOptions
	output              string
}

func newGetFunctionCommandeer(getCommandeer *getCommandeer) *getFunctionCommandeer {
	commandeer := &getFunctionCommandeer{
		getCommandeer: getCommandeer,
	}

	cmd := &cobra.Command{
		Use:     "function [name[:version]]",
		Aliases: []string{"fu"},
		Short:   "Display function information",
		RunE: func(cmd *cobra.Command, args []string) error {
			commandeer.getFunctionsOptions.Namespace = getCommandeer.rootCommandeer.namespace

			// if we got positional arguments
			if len(args) != 0 {

				// second argument is a resource name
				commandeer.getFunctionsOptions.Name = args[0]
			}

			// initialize root
			if err := getCommandeer.rootCommandeer.initialize(); err != nil {
				return errors.Wrap(err, "Failed to initialize root")
			}

			functions, err := getCommandeer.rootCommandeer.platform.GetFunctions(&commandeer.getFunctionsOptions)
			if err != nil {
				return errors.Wrap(err, "Failed to get functions")
			}

			if len(functions) == 0 {
				cmd.OutOrStdout().Write([]byte("No functions found"))
				return nil
			}

			// render the functions
			return commandeer.renderFunctions(functions, commandeer.output, cmd.OutOrStdout())
		},
	}

	cmd.PersistentFlags().StringVarP(&commandeer.getFunctionsOptions.Labels, "labels", "l", "", "Function labels (lbl1=val1[,lbl2=val2,...])")
	cmd.PersistentFlags().StringVarP(&commandeer.output, "output", "o", outputFormatText, "Output format - \"text\", \"wide\", \"yaml\", or \"json\"")

	commandeer.cmd = cmd

	return commandeer
}

func (g *getFunctionCommandeer) renderFunctions(functions []platform.Function, format string, writer io.Writer) error {

	// iterate over each function and make sure it's initialized
	// TODO: parallelize
	for _, function := range functions {
		if err := function.Initialize(nil); err != nil {
			return err
		}
	}

	rendererInstance := renderer.NewRenderer(writer)

	switch format {
	case outputFormatText, outputFormatWide:
		header := []string{"Namespace", "Name", "Project", "State", "Node Port", "Replicas"}
		if format == outputFormatWide {
			header = append(header, []string{
				"Labels",
				"Ingresses",
			}...)
		}

		functionRecords := [][]string{}

		// for each field
		for _, function := range functions {
			availableReplicas, specifiedReplicas := function.GetReplicas()

			// get its fields
			functionFields := []string{
				function.GetConfig().Meta.Namespace,
				function.GetConfig().Meta.Name,
				function.GetConfig().Meta.Labels["nuclio.io/project-name"],
				string(function.GetStatus().State),
				strconv.Itoa(function.GetConfig().Spec.HTTPPort),
				fmt.Sprintf("%d/%d", availableReplicas, specifiedReplicas),
			}

			// add fields for wide view
			if format == outputFormatWide {
				functionFields = append(functionFields, []string{
					common.StringMapToString(function.GetConfig().Meta.Labels),
					g.formatFunctionIngresses(function),
				}...)
			}

			// add to records
			functionRecords = append(functionRecords, functionFields)
		}

		rendererInstance.RenderTable(header, functionRecords)
	case "yaml":
		g.renderFunctionConfig(functions, rendererInstance.RenderYAML)
	case "json":
		g.renderFunctionConfig(functions, rendererInstance.RenderJSON)
	}

	return nil
}

func (g *getFunctionCommandeer) formatFunctionIngresses(function platform.Function) string {
	var formattedIngresses string

	ingresses := function.GetIngresses()

	for _, ingress := range ingresses {
		host := ingress.Host
		if host != "" {
			host += ":<port>"
		}

		for _, path := range ingress.Paths {
			formattedIngresses += fmt.Sprintf("%s%s, ", host, path)
		}
	}

	// add default ingress
	formattedIngresses += fmt.Sprintf("/%s/%s",
		function.GetConfig().Meta.Name,
		function.GetVersion())

	return formattedIngresses
}

func (g *getFunctionCommandeer) renderFunctionConfig(functions []platform.Function, renderer func(interface{}) error) error {
	for _, function := range functions {
		if err := renderer(function.GetConfig()); err != nil {
			return errors.Wrap(err, "Failed to render function config")
		}

	}

	return nil
}

type getProjectCommandeer struct {
	*getCommandeer
	getProjectsOptions platform.GetProjectsOptions
	output             string
}

func newGetProjectCommandeer(getCommandeer *getCommandeer) *getProjectCommandeer {
	commandeer := &getProjectCommandeer{
		getCommandeer: getCommandeer,
	}

	cmd := &cobra.Command{
		Use:     "project name",
		Aliases: []string{"proj"},
		Short:   "Display project information",
		RunE: func(cmd *cobra.Command, args []string) error {
			commandeer.getProjectsOptions.Meta.Namespace = getCommandeer.rootCommandeer.namespace

			// if we got positional arguments
			if len(args) != 0 {

				// second argument is a resource name
				commandeer.getProjectsOptions.Meta.Name = args[0]
			}

			// initialize root
			if err := getCommandeer.rootCommandeer.initialize(); err != nil {
				return errors.Wrap(err, "Failed to initialize root")
			}

			projects, err := getCommandeer.rootCommandeer.platform.GetProjects(&commandeer.getProjectsOptions)
			if err != nil {
				return errors.Wrap(err, "Failed to get projects")
			}

			if len(projects) == 0 {
				cmd.OutOrStdout().Write([]byte("No projects found"))
				return nil
			}

			// render the projects
			return commandeer.renderProjects(projects, commandeer.output, cmd.OutOrStdout())
		},
	}

	cmd.PersistentFlags().StringVarP(&commandeer.output, "output", "o", outputFormatText, "Output format - \"text\", \"wide\", \"yaml\", or \"json\"")

	commandeer.cmd = cmd

	return commandeer
}

func (g *getProjectCommandeer) renderProjects(projects []platform.Project, format string, writer io.Writer) error {

	rendererInstance := renderer.NewRenderer(writer)

	switch format {
	case outputFormatText, outputFormatWide:
		header := []string{"Namespace", "Name", "Display Name"}
		if format == outputFormatWide {
			header = append(header, []string{
				"Description",
			}...)
		}

		var projectRecords [][]string

		// for each field
		for _, project := range projects {

			// get its fields
			projectFields := []string{
				project.GetConfig().Meta.Namespace,
				project.GetConfig().Meta.Name,
				project.GetConfig().Spec.DisplayName,
			}

			// add fields for wide view
			if format == outputFormatWide {
				projectFields = append(projectFields, []string{
					project.GetConfig().Spec.Description,
				}...)
			}

			// add to records
			projectRecords = append(projectRecords, projectFields)
		}

		rendererInstance.RenderTable(header, projectRecords)
	case "yaml":
		g.renderProjectConfig(projects, rendererInstance.RenderYAML)
	case "json":
		g.renderProjectConfig(projects, rendererInstance.RenderJSON)
	}

	return nil
}

func (g *getProjectCommandeer) renderProjectConfig(projects []platform.Project, renderer func(interface{}) error) error {
	for _, project := range projects {
		if err := renderer(project.GetConfig()); err != nil {
			return errors.Wrap(err, "Failed to render project config")
		}
	}

	return nil
}
