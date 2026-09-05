//go:build windows

package main

import (
	"errors"
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
	Flags  string   `json:"flags"`
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
		return NewError(err)
	}
	defer key.Close()
	if err := key.SetExpandStringValue(REG_EXE, c.Exe); err != nil {
		return NewError(err)
	}
	if err := key.SetExpandStringValue(REG_FLAGS, c.Flags); err != nil {
		return NewError(err)
	}
	if err := key.SetExpandStringValue(REG_DIR, c.Dir); err != nil {
		return NewError(err)
	}
	if err := key.SetExpandStringValue(REG_STDIN, c.Stdin); err != nil {
		return NewError(err)
	}
	if err := key.SetExpandStringValue(REG_STDOUT, c.Stdout); err != nil {
		return NewError(err)
	}
	if err := key.SetExpandStringValue(REG_STDERR, c.Stderr); err != nil {
		return NewError(err)
	}
	if err := key.SetStringsValue(REG_ENV, c.Env); err != nil {
		return NewError(err)
	}
	return nil
}

func GetConfig(name string) (*Config, error) {
	path := fmt.Sprintf("%s\\%s\\%s", REGISTRY, name, REG_PARAMETERS)
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
	if err != nil {
		return nil, NewError(err)
	}
	defer key.Close()
	var c Config
	if c.Exe, _, err = key.GetStringValue(REG_EXE); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	if c.Flags, _, err = key.GetStringValue(REG_FLAGS); err != nil {
		if errors.Is(err, registry.ErrUnexpectedType) {
			if flags, _, err := key.GetStringsValue(REG_FLAGS); err == nil {
				c.Flags = Join(flags...)
			} else {
				return nil, NewError(err)
			}
		} else if !errors.Is(err, registry.ErrNotExist) {
			return nil, NewError(err)
		}
	}
	if c.Dir, _, err = key.GetStringValue(REG_DIR); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	if c.Stdin, _, err = key.GetStringValue(REG_STDIN); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	if c.Stdout, _, err = key.GetStringValue(REG_STDOUT); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	if c.Stderr, _, err = key.GetStringValue(REG_STDERR); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	if c.Env, _, err = key.GetStringsValue(REG_ENV); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, NewError(err)
	}
	return &c, nil
}
