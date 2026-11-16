// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package nfregistration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omec-project/ausf/consumer"
	"github.com/omec-project/openapi/v2/models"
)

// The registration logic is tested in openapi/nrfregistration. These tests check that the AUSF wiring
// reaches the right consumer functions.

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestStartNfRegistrationService_NonEmptyConfigRegistersThenEmptyConfigDeregisters(t *testing.T) {
	origRegister, origDeregister := consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance
	t.Cleanup(func() {
		consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance = origRegister, origDeregister
	})
	var registeredWith atomic.Value
	var deregistered atomic.Int32
	consumer.SendRegisterNFInstance = func(_ context.Context, plmnConfig []models.PlmnId) (*models.NFProfile, string, error) {
		registeredWith.Store(plmnConfig)
		return models.NewNFProfileWithDefaults(), "", nil
	}
	consumer.SendDeregisterNFInstance = func(context.Context) error {
		deregistered.Add(1)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ch := make(chan []models.PlmnId)
	go func() { defer close(done); StartNfRegistrationService(ctx, ch) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	plmns := []models.PlmnId{*models.NewPlmnId("123", "45")}
	ch <- plmns
	waitFor(t, func() bool { return registeredWith.Load() != nil }, "SendRegisterNFInstance was not called")
	if got := registeredWith.Load().([]models.PlmnId); len(got) != 1 || got[0].Mcc != "123" {
		t.Errorf("registered with %+v", got)
	}

	ch <- nil
	waitFor(t, func() bool { return deregistered.Load() == 1 }, "SendDeregisterNFInstance was not called for an empty config")
}

func TestDeregisterNF_CallsConsumer(t *testing.T) {
	orig := consumer.SendDeregisterNFInstance
	t.Cleanup(func() { consumer.SendDeregisterNFInstance = orig })
	var called atomic.Int32
	consumer.SendDeregisterNFInstance = func(context.Context) error { called.Add(1); return nil }

	DeregisterNF()
	if called.Load() != 1 {
		t.Errorf("SendDeregisterNFInstance calls = %d, want 1", called.Load())
	}
}

func TestHeartbeatUsesSendUpdateNFInstance(t *testing.T) {
	origRegister, origUpdate := consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance
	t.Cleanup(func() { consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance = origRegister, origUpdate })
	var updated atomic.Int32
	profile := models.NewNFProfileWithDefaults()
	profile.SetHeartBeatTimer(1)
	consumer.SendRegisterNFInstance = func(context.Context, []models.PlmnId) (*models.NFProfile, string, error) { return profile, "", nil }
	consumer.SendUpdateNFInstance = func(_ context.Context, patch []models.PatchItem) (*models.NFProfile, *models.ProblemDetails, error) {
		updated.Add(1)
		return profile, nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ch := make(chan []models.PlmnId)
	go func() { defer close(done); StartNfRegistrationService(ctx, ch) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	ch <- []models.PlmnId{*models.NewPlmnId("123", "45")}
	waitFor(t, func() bool { return updated.Load() >= 1 }, "heartbeat did not call SendUpdateNFInstance")
}

// A registration that the NRF does not answer holds the registration service's mutex, and the service waits for
// it before it deregisters. Clearing the PLMN configuration cancels the registration's context, which has to
// reach the request, or the NF would never deregister.
func TestEmptyConfigEndsARegistrationThatIsNotAnswered(t *testing.T) {
	origRegister, origDeregister := consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance
	t.Cleanup(func() {
		consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance = origRegister, origDeregister
	})
	started := make(chan struct{}, 1)
	var cancelled, deregistered atomic.Bool
	consumer.SendRegisterNFInstance = func(ctx context.Context, _ []models.PlmnId) (*models.NFProfile, string, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done() // the NRF never answers
		cancelled.Store(true)
		return nil, "", ctx.Err()
	}
	consumer.SendDeregisterNFInstance = func(context.Context) error { deregistered.Store(true); return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ch := make(chan []models.PlmnId)
	go func() { defer close(done); StartNfRegistrationService(ctx, ch) }()
	t.Cleanup(func() { cancel(); <-done })

	ch <- []models.PlmnId{*models.NewPlmnId("123", "45")}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the registration was not sent")
	}
	ch <- nil
	waitFor(t, deregistered.Load, "the NF was not deregistered while a registration was not being answered")
	if !cancelled.Load() {
		t.Error("the registration request did not see its context cancelled")
	}
}

// DeregisterNF is bounded: an NRF that does not answer the DELETE must not hold up the termination of the NF.
func TestDeregisterNFGivesUpOnAnNrfThatDoesNotAnswer(t *testing.T) {
	orig, origTimeout := consumer.SendDeregisterNFInstance, deregisterTimeout
	t.Cleanup(func() { consumer.SendDeregisterNFInstance, deregisterTimeout = orig, origTimeout })
	deregisterTimeout = 100 * time.Millisecond
	consumer.SendDeregisterNFInstance = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}

	finished := make(chan struct{})
	go func() { DeregisterNF(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("DeregisterNF did not return for an NRF that does not answer")
	}
}
