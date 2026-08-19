//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	REGISTRY       = "SYSTEM\\CurrentControlSet\\Services"
	REG_PARAMETERS = "Parameters"
	REG_EXE        = "Application"
	REG_FLAGS      = "AppParameters"
	REG_DIR        = "AppDirectory"
	REG_STDIN      = "AppStdin"
	REG_STDOUT     = "AppStdout"
	REG_STDERR     = "AppStderr"
	REG_ENV        = "AppEnvironment"
)

type Config struct {
	Exe    string   `json:"exe"`
	Flags  []string `json:"flags"`
	Dir    string   `json:"dir"`
	Stdin  string   `json:"stdin"`
	Stdout string   `json:"stdout"`
	Stderr string   `json:"stderr"`
	Env    []string `json:"env"`
}

func SetConfig(name string, c Config) error {
	path := fmt.Sprintf("%s\\%s\\%s", REGISTRY, name, REG_PARAMETERS)
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, path, registry.WRITE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetExpandStringValue(REG_EXE, c.Exe); err != nil {
		return err
	}
	if err := key.SetStringsValue(REG_FLAGS, c.Flags); err != nil {
		return err
	}
	if err := key.SetExpandStringValue(REG_DIR, c.Dir); err != nil {
		return err
	}
	if err := key.SetExpandStringValue(REG_STDIN, c.Stdin); err != nil {
		return err
	}
	if err := key.SetExpandStringValue(REG_STDOUT, c.Stdout); err != nil {
		return err
	}
	if err := key.SetExpandStringValue(REG_STDERR, c.Stderr); err != nil {
		return err
	}
	if err := key.SetStringsValue(REG_ENV, c.Env); err != nil {
		return err
	}
	return nil
}

func GetConfig(name string) (*Config, error) {
	path := fmt.Sprintf("%s\\%s\\%s", REGISTRY, name, REG_PARAMETERS)
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
	if err != nil {
		return nil, err
	}
	defer key.Close()
	var c Config
	if c.Exe, _, err = key.GetStringValue(REG_EXE); err != nil {
		return nil, err
	}
	if c.Flags, _, err = key.GetStringsValue(REG_FLAGS); err != nil {
		return nil, err
	}
	if c.Dir, _, err = key.GetStringValue(REG_DIR); err != nil {
		return nil, err
	}
	if c.Stdin, _, err = key.GetStringValue(REG_STDIN); err != nil {
		return nil, err
	}
	if c.Stdout, _, err = key.GetStringValue(REG_STDOUT); err != nil {
		return nil, err
	}
	if c.Stderr, _, err = key.GetStringValue(REG_STDERR); err != nil {
		return nil, err
	}
	if c.Env, _, err = key.GetStringsValue(REG_ENV); err != nil {
		return nil, err
	}
	return &c, nil
}
