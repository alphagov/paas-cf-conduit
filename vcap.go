package main

type Env struct {
	SystemEnvJSON SystemEnv `json:"system_env_json"`
}

type SystemEnv struct {
	VcapServices map[string][]Service `json:"VCAP_SERVICES"`
}

type Service map[string]interface{}

type Credentials struct {
	Host     string `json:"host"`
	Port     int64  `json:"port"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}
