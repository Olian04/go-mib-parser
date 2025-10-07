package mibs_test

import (
    "testing"

    "github.com/Olian04/go-mib-parser/tests/testutil"
)

func Test_IF_MIB_ParseAndContents(t *testing.T) {
    src := testutil.ReadMIB(t, "IF-MIB.MIB")
    testutil.VerifyMIB(t, src, "IF-MIB.MIB")
}


