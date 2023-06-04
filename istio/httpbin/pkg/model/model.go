package model

type Response struct {
	Args     map[string]string `json:"args"`
	Form     map[string]string `json:"form"`
	Headers  map[string]string `json:"headers"`
	Method   string            `json:"method"`
	Origin   string            `json:"origin"`
	Url      string            `json:"url"`
	Envs     map[string]string `json:"envs"`
	HostName string            `json:"host_name"`
	Body     string            `json:"body"`
}

type ResponseAny struct {
	Code int `json:"code"`
	Data any `json:"data"`
}

type Base struct {
	Application    string `json:"application" yaml:"application"`
	Service        string `json:"service" yaml:"service"`
	ID             string `json:"id" yaml:"id"`
	ServiceVersion string `json:"serviceVersion" yaml:"serviceVersion"`
	ServiceGroup   string `json:"serviceGroup" yaml:"serviceGroup"`
}

type ConditionRouteDto struct {
	Base

	Conditions []string `json:"conditions" yaml:"conditions" binding:"required"`

	Priority      int    `json:"priority" yaml:"priority"`
	Enabled       bool   `json:"enabled" yaml:"enabled" binding:"required"`
	Force         bool   `json:"force" yaml:"force"`
	Runtime       bool   `json:"runtime" yaml:"runtime"`
	ConfigVersion string `json:"configVersion" yaml:"configVersion" binding:"required"`
}
