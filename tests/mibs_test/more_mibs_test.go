package mibs_test

import (
    "testing"

    "github.com/Olian04/go-mib-parser/tests/testutil"
)

func Test_ENTITY_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "ENTITY-MIB.mib")
    testutil.VerifyMIB(t, src, "ENTITY-MIB.mib")
}

func Test_INET_ADDRESS_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "INET-ADDRESS-MIB.mib")
    testutil.VerifyMIB(t, src, "INET-ADDRESS-MIB.mib")
}

func Test_IANAifType_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "IANAifType-MIB.mib")
    testutil.VerifyMIB(t, src, "IANAifType-MIB.mib")
}

func Test_IP_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "IP-MIB.MIB")
    testutil.VerifyMIB(t, src, "IP-MIB.MIB")
}


