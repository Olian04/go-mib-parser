package mibs_test

import (
    "testing"

    "github.com/Olian04/go-mib-parser/tests/testutil"
)

func Test_SNMP_FRAMEWORK_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMP-FRAMEWORK-MIB.mib")
    testutil.VerifyMIB(t, src, "SNMP-FRAMEWORK-MIB.mib")
}

func Test_SNMP_NOTIFICATION_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMP-NOTIFICATION-MIB.mib")
    testutil.VerifyMIB(t, src, "SNMP-NOTIFICATION-MIB.mib")
}

func Test_SNMP_TARGET_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMP-TARGET-MIB.mib")
    testutil.VerifyMIB(t, src, "SNMP-TARGET-MIB.mib")
}

func Test_SNMPv2_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "SNMPv2-MIB.mib")
    testutil.VerifyMIB(t, src, "SNMPv2-MIB.mib")
}


