package mibs_test

import (
    "testing"

    "github.com/Olian04/go-mib-parser/tests/testutil"
)

func Test_SNMPv2_SMI_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMPv2-SMI.mib")
    testutil.VerifyMIB(t, src, "SNMPv2-SMI.mib")
}

func Test_SNMPv2_CONF_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMPv2-CONF.mib")
    testutil.VerifyMIB(t, src, "SNMPv2-CONF.mib")
}

func Test_SNMPV2_TC_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMPV2-TC.mib")
    testutil.VerifyMIB(t, src, "SNMPV2-TC.mib")
}


