package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/dwarvesf/qor"
	"github.com/dwarvesf/qor/utils"
)

// Context admin context, which is used for admin controller
type Context struct {
	*qor.Context
	*Searcher
	Flashes  []Flash
	Resource *Resource
	Admin    *Admin
	Content  template.HTML
	Action   string
	Settings map[string]interface{}
	Result   interface{}

	funcMaps template.FuncMap
}

// NewContext new admin context
func (admin *Admin) NewContext(w http.ResponseWriter, r *http.Request) *Context {
	return &Context{Context: &qor.Context{Config: admin.Config, Request: r, Writer: w}, Admin: admin, Settings: map[string]interface{}{}}
}

// Funcs set FuncMap for templates
func (context *Context) Funcs(funcMaps template.FuncMap) *Context {
	if context.funcMaps == nil {
		context.funcMaps = template.FuncMap{}
	}

	for key, value := range funcMaps {
		context.funcMaps[key] = value
	}
	return context
}

func (context *Context) clone() *Context {
	return &Context{
		Context:  context.Context,
		Searcher: context.Searcher,
		Flashes:  context.Flashes,
		Resource: context.Resource,
		Admin:    context.Admin,
		Result:   context.Result,
		Content:  context.Content,
		Settings: context.Settings,
		Action:   context.Action,
		funcMaps: context.funcMaps,
	}
}

// Get get context's Settings
func (context *Context) Get(key string) interface{} {
	return context.Settings[key]
}

// Set set context's Settings
func (context *Context) Set(key string, value interface{}) {
	context.Settings[key] = value
}

func (context *Context) resourcePath() string {
	if context.Resource == nil {
		return ""
	}
	return context.Resource.ToParam()
}

func (context *Context) setResource(res *Resource) *Context {
	if res != nil {
		context.Resource = res
		context.ResourceID = res.GetPrimaryValue(context.Request)
	}
	context.Searcher = &Searcher{Context: context}
	return context
}

func (context *Context) Asset(layouts ...string) ([]byte, error) {
	var prefixes, themes []string

	if context.Request != nil {
		if theme := context.Request.URL.Query().Get("theme"); theme != "" {
			themes = append(themes, theme)
		}
	}

	if len(themes) == 0 && context.Resource != nil {
		for _, theme := range context.Resource.Config.Themes {
			themes = append(themes, theme.GetName())
		}
	}

	if resourcePath := context.resourcePath(); resourcePath != "" {
		for _, theme := range themes {
			prefixes = append(prefixes, filepath.Join("themes", theme, resourcePath))
		}
		prefixes = append(prefixes, resourcePath)
	}

	for _, theme := range themes {
		prefixes = append(prefixes, filepath.Join("themes", theme))
	}

	for _, layout := range layouts {
		for _, prefix := range prefixes {
			if content, err := context.Admin.AssetFS.Asset(filepath.Join(prefix, layout)); err == nil {
				return content, nil
			}
		}

		if content, err := context.Admin.AssetFS.Asset(layout); err == nil {
			return content, nil
		}
	}

	return []byte(""), fmt.Errorf("template not found: %v", layouts)
}

// renderText render text based on data
func (context *Context) renderText(text string, data interface{}) template.HTML {
	var (
		err    error
		tmpl   *template.Template
		result = bytes.NewBufferString("")
	)

	if tmpl, err = template.New("").Funcs(context.FuncMap()).Parse(text); err == nil {
		if err = tmpl.Execute(result, data); err == nil {
			return template.HTML(result.String())
		}
	}

	return template.HTML(err.Error())
}

// renderWith render template based on data
func (context *Context) renderWith(name string, data interface{}) template.HTML {
	var (
		err     error
		content []byte
	)

	if content, err = context.Asset(name + ".tmpl"); err == nil {
		return context.renderText(string(content), data)
	}
	return template.HTML(err.Error())
}

// Render render template based on context
func (context *Context) Render(name string, results ...interface{}) template.HTML {
	defer func() {
		if r := recover(); r != nil {
			err := errors.New(fmt.Sprintf("Get error when render file %v: %v", name, r))
			utils.ExitWithMsg(err)
		}
	}()

	clone := context.clone()
	if len(results) > 0 {
		clone.Result = results[0]
	}

	return clone.renderWith(name, clone)
}

// Execute execute template with layout
func (context *Context) Execute(name string, result interface{}) {
	var tmpl *template.Template

	if name == "show" && !context.Resource.isSetShowAttrs {
		name = "edit"
	}

	if context.Action == "" {
		context.Action = name
	}

	if content, err := context.Asset("layout.tmpl"); err == nil {
		if tmpl, err = template.New("layout").Funcs(context.FuncMap()).Parse(string(content)); err == nil {
			for _, name := range []string{"header", "footer"} {
				if tmpl.Lookup(name) == nil {
					if content, err := context.Asset(name + ".tmpl"); err == nil {
						tmpl.Parse(string(content))
					}
				} else {
					utils.ExitWithMsg(err)
				}
			}
		} else {
			utils.ExitWithMsg(err)
		}
	}

	context.Result = result
	context.Content = context.Render(name, result)
	if err := tmpl.Execute(context.Writer, context); err != nil {
		utils.ExitWithMsg(err)
	}
}

// JSON generate json outputs for action
func (context *Context) JSON(action string, result interface{}) {
	if action == "show" && !context.Resource.isSetShowAttrs {
		action = "edit"
	}

	js, err := json.MarshalIndent(context.Resource.convertObjectToJSONMap(context, result, action), "", "\t")
	context.Writer.Header().Set("Content-Type", "application/json")
	if err != nil {
		result := make(map[string]string)
		result["error"] = err.Error()
		js, _ = json.Marshal(result)
	}
	context.Writer.Write(js)
}
