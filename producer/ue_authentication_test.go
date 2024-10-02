// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Canonical Ltd.

package producer_test

import (
	"testing"

	"github.com/omec-project/ausf/producer"
)

func TestGenerateRandomNumber(t *testing.T) {
	value, err := producer.GenerateRandomNumber()

	if err != nil {
		t.Fatalf("GenerateRandomNumber() failed: %s", err)
	}

	t.Logf("random number: %d", value)
}
