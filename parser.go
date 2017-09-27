package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/blang/semver"
	"github.com/rancher/catalog-service/model"
	"github.com/rancher/catalog-service/parse"
	"github.com/rancher/catalog-service/utils"
)

func getTemplates(ctx *cli.Context) error {

	branch := ctx.String("branch")
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repoPath, err := ioutil.TempDir(wd, "rancher-catalog-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(repoPath)
	var cmd *exec.Cmd
	if branch == "" {
		cmd = exec.Command("git", "clone", "https://github.com/rancher/rancher-catalog.git", repoPath)
	} else {
		cmd = exec.Command("git", "clone", "-b", branch, "--single-branch", "https://github.com/rancher/rancher-catalog.git", repoPath)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	//intraPath := "/Users/lipinghui/Workspace/go/src/github.com/gitlawr/infra-version/catalog-871074990"
	_, errs, err := traverseCatalogFiles(repoPath)
	if len(errs) != 1 && err != nil {
		logrus.Errorf("%v,%v", errs, err)
	}
	return nil
}

func traverseCatalogFiles(repoPath string) ([]model.Template, []error, error) {
	templateIndex := map[string]*model.Template{}
	var errors []error

	if err := filepath.Walk(repoPath, func(fullPath string, f os.FileInfo, err error) error {
		if f == nil || !f.Mode().IsRegular() {
			return nil
		}

		relativePath, err := filepath.Rel(repoPath, fullPath)
		if err != nil {
			return err
		}

		_, _, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		_, filename := path.Split(relativePath)

		if err = handleFile(templateIndex, fullPath, relativePath, filename); err != nil {
			errors = append(errors, fmt.Errorf("%s: %v", fullPath, err))
		}

		return nil
	}); err != nil {
		return nil, nil, err
	}

	templates := []model.Template{}
	for _, template := range templateIndex {
		for i, version := range template.Versions {
			var readme string
			for _, file := range version.Files {
				if strings.ToLower(file.Name) == "readme.md" {
					readme = file.Contents
				}
			}
			var rancherCompose string
			var templateVersion string
			for _, file := range version.Files {

				if file.Name == "rancher-compose.yml" {
					rancherCompose = file.Contents
				}

				if file.Name == "template-version.yml" {
					templateVersion = file.Contents
				}

			}
			newVersion := version
			if rancherCompose != "" || templateVersion != "" {

				var err error
				if rancherCompose != "" {
					newVersion, err = parse.CatalogInfoFromRancherCompose([]byte(rancherCompose))
				}

				if templateVersion != "" {
					newVersion, err = parse.CatalogInfoFromTemplateVersion([]byte(templateVersion))
				}

				if err != nil {
					var id string
					if template.Base == "" {
						id = fmt.Sprintf("%s:%d", template.FolderName, i)
					} else {
						id = fmt.Sprintf("%s*%s:%d", template.Base, template.FolderName, i)
					}
					errors = append(errors, fmt.Errorf("Failed to parse rancher-compose.yml for %s: %v", id, err))
					continue
				}
				newVersion.Revision = version.Revision
				// If rancher-compose.yml contains version, use this instead of folder version
				if newVersion.Version == "" {
					newVersion.Version = version.Version
				}
				newVersion.Files = version.Files
			}
			newVersion.Readme = readme

			template.Versions[i] = newVersion
		}
		var filteredVersions []model.Version
		for _, version := range template.Versions {
			if version.Version != "" {
				if utils.VersionBetween(version.MinimumRancherVersion, RANCHERVERSION, version.MaximumRancherVersion) {
					filteredVersions = append(filteredVersions, version)
					if template.Base == "infra" {
						fmt.Printf("template:%s\n", template.Name)
						fmt.Printf("revision:%d\n", *version.Revision)
						fmt.Printf("version:%s\n", version.Version)
						for _, f := range version.Files {
							if strings.HasPrefix(strings.ToLower(f.Name), "docker-compose") {
								r, _ := regexp.Compile(" image:(.*?)\n")
								images := r.FindAllString(f.Contents, -1)
								RemoveDuplicates(&images)
								fmt.Println(images)
							}
						}
						fmt.Println("")
					}
				}
			}
		}
		template.Versions = filteredVersions

		templates = append(templates, *template)

	}

	return templates, errors, nil
}

func handleFile(templateIndex map[string]*model.Template, fullPath, relativePath, filename string) error {
	switch {
	case filename == "config.yml" || filename == "template.yml":
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}
		contents, err := ioutil.ReadFile(fullPath)
		if err != nil {
			return err
		}

		var template model.Template
		if template, err = parse.TemplateInfo(contents); err != nil {
			return err
		}

		template.Base = base
		template.FolderName = templateName

		key := base + templateName

		if existingTemplate, ok := templateIndex[key]; ok {
			template.Icon = existingTemplate.Icon
			template.IconFilename = existingTemplate.IconFilename
			template.Readme = existingTemplate.Readme
			template.Versions = existingTemplate.Versions
		}
		templateIndex[key] = &template
	case strings.HasPrefix(filename, "catalogIcon") || strings.HasPrefix(filename, "icon"):
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		contents, err := ioutil.ReadFile(fullPath)
		if err != nil {
			return err
		}

		key := base + templateName

		if _, ok := templateIndex[key]; !ok {
			templateIndex[key] = &model.Template{}
		}
		templateIndex[key].Icon = base64.StdEncoding.EncodeToString([]byte(contents))
		templateIndex[key].IconFilename = filename
	case strings.HasPrefix(strings.ToLower(filename), "readme.md"):
		base, templateName, parsedCorrectly := parse.TemplatePath(relativePath)
		if !parsedCorrectly {
			return nil
		}

		_, _, _, parsedCorrectly = parse.VersionPath(relativePath)
		if parsedCorrectly {
			return handleVersionFile(templateIndex, fullPath, relativePath, filename)
		}

		contents, err := ioutil.ReadFile(fullPath)
		if err != nil {
			return err
		}

		key := base + templateName

		if _, ok := templateIndex[key]; !ok {
			templateIndex[key] = &model.Template{}
		}
		templateIndex[key].Readme = string(contents)
	default:
		return handleVersionFile(templateIndex, fullPath, relativePath, filename)
	}

	return nil
}

func handleVersionFile(templateIndex map[string]*model.Template, fullPath, relativePath, filename string) error {
	base, templateName, folderName, parsedCorrectly := parse.VersionPath(relativePath)
	if !parsedCorrectly {
		return nil
	}
	contents, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return err
	}

	key := base + templateName
	file := model.File{
		Name:     filename,
		Contents: string(contents),
	}

	if _, ok := templateIndex[key]; !ok {
		templateIndex[key] = &model.Template{}
	}

	// Handle case where folder name is a revision (just a number)
	revision, err := strconv.Atoi(folderName)
	if err == nil {
		for i, version := range templateIndex[key].Versions {
			if version.Revision != nil && *version.Revision == revision {
				templateIndex[key].Versions[i].Files = append(version.Files, file)
				return nil
			}
		}
		templateIndex[key].Versions = append(templateIndex[key].Versions, model.Version{
			Revision: &revision,
			Files:    []model.File{file},
		})
		return nil
	}

	// Handle case where folder name is version (must be in semver format)
	_, err = semver.Parse(strings.Trim(folderName, "v"))
	if err == nil {
		for i, version := range templateIndex[key].Versions {
			if version.Version == folderName {
				templateIndex[key].Versions[i].Files = append(version.Files, file)
				return nil
			}
		}
		templateIndex[key].Versions = append(templateIndex[key].Versions, model.Version{
			Version: folderName,
			Files:   []model.File{file},
		})
		return nil
	}

	return nil
}

func RemoveDuplicates(xs *[]string) {
	found := make(map[string]bool)
	j := 0
	for i, x := range *xs {
		if !found[x] {
			found[x] = true
			(*xs)[j] = (*xs)[i]
			j++
		}
	}
	*xs = (*xs)[:j]
}
