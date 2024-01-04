package producer_test

import (
	"testing"

	"github.com/omec-project/ausf/producer"
)

func TestGenerateRandomNumber(t *testing.T) {
	randomNumber, err := producer.GenerateRandomNumber()

	if err != nil {
		t.Fatalf("GenerateRandomNumber() failed: %s", err)
	}

	if randomNumber < 0 || randomNumber > 255 {
		t.Fatalf("GenerateRandomNumber() failed: %d is not between 0 and 255", randomNumber)
	}

}
