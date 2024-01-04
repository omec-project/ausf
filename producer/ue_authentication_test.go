package producer_test

import (
	"fmt"
	"testing"

	"github.com/omec-project/ausf/producer"
)

func TestGenerateRandomNumber(t *testing.T) {
	value, err := producer.GenerateRandomNumber()

	if err != nil {
		t.Fatalf("GenerateRandomNumber() failed: %s", err)
	}

	fmt.Printf("Random number: %d\n", value)

}
