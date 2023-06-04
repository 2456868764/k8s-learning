package utils

import (
	"os"
	"strings"
)

const (
	EnvServiceName = "SERVICE_NAME"
	EnvPodName     = "POD_NAME"
	EnvSubSystem   = "SUB_SYSTEM"
	EnvNameSpace   = "POD_NAMESPACE"
)

func GetAllEnvs() map[string]string {
	allEnvs := make(map[string]string, 2)
	envs := os.Environ()
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		} else {
			allEnvs[parts[0]] = parts[1]
		}
	}
	return allEnvs
}

func GetHostName() string {
	return GetStringEnv(EnvPodName, GetDefaultHostName())
}

func GetNameSpace() string {
	return GetStringEnv(EnvNameSpace, "")
}

func GetServiceName() string {
	return GetStringEnv(EnvServiceName, GetDefaultHostName())
}

func GetSubSystem() string {
	return GetStringEnv(EnvSubSystem, "")
}

func GetDefaultHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}

func GetStringEnv(name string, defvalue string) string {
	val, ex := os.LookupEnv(name)
	if ex {
		return val
	} else {
		return defvalue
	}
}

func FileExisted(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return true
	}
	return true
}
