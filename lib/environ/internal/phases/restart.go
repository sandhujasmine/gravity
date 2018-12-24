/*
Copyright 2018 Gravitational, Inc.

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

package phases

import (
	"bytes"
	"context"
	"os/exec"

	libfsm "github.com/gravitational/gravity/lib/fsm"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/utils"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// NewRestart returns a new executor to restart the runtime container to apply
// the environment variables update
func NewRestart(params libfsm.ExecutorParams, apps appGetter, logger log.FieldLogger) (*restart, error) {
	if params.Phase.Data == nil || params.Phase.Data.Package == nil {
		return nil, trace.NotFound("no installed application package specified for phase %q",
			params.Phase.ID)
	}
	app, err := apps.GetApp(*params.Phase.Data.Package)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query installed application")
	}
	runtimePackage, err := app.Manifest.RuntimePackageForProfile(params.Phase.Data.Server.Role)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &restart{
		FieldLogger:    logger,
		runtimePackage: *runtimePackage,
	}, nil
}

// Execute restarts the runtime container with the new configuration package
func (r *restart) Execute(ctx context.Context) error {
	commands := [][]string{
		{"system", "pull-updates", "--runtime-package", r.runtimePackage.String()},
		{"system", "update"},
	}
	for _, command := range commands {
		args := utils.Self(command...)
		cmd := exec.Command(args[0], args[1:]...)
		var buf bytes.Buffer
		err := utils.ExecL(cmd, &buf, r.FieldLogger)
		if err != nil {
			r.WithField("output", buf.String()).
				Warn("Failed to execute.")
			return trace.Wrap(err)
		}
	}
	return nil
}

// Rollback reverses the update and restarts the container with the old
// configuration package
func (r *restart) Rollback(context.Context) error {
	args := utils.Self("system", "rollback")
	cmd := exec.Command(args[0], args[1:]...)
	var buf bytes.Buffer
	err := utils.ExecL(cmd, &buf, r.FieldLogger)
	if err != nil {
		r.WithField("output", buf.String()).
			Warn("Failed to execute.")
		return trace.Wrap(err)
	}
	return nil
}

// PreCheck is a no-op
func (*restart) PreCheck(context.Context) error {
	return nil
}

// PostCheck is a no-op
func (*restart) PostCheck(context.Context) error {
	return nil
}

type restart struct {
	// FieldLogger specifies the logger for the phase
	log.FieldLogger
	runtimePackage loc.Locator
}
