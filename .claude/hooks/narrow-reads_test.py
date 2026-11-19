"""Tests for narrow-reads.py. Run: python3 -I -m unittest discover -s .claude/hooks -p '*_test.py'"""
import importlib.util
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

HOOK = pathlib.Path(__file__).with_name("narrow-reads.py")
_spec = importlib.util.spec_from_file_location("narrow_reads", HOOK)
narrow_reads = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(narrow_reads)

GO = """package x

import "fmt"

type Queue struct {
	a int
}

func (q *Queue) Push() {
	fmt.Println()
}

func helper() {}

var (
	one = 1
)

const Max = 3

func main() {
	x := 1
}
"""

TS = """import a from "a";
export function foo() {}
function private() {}
export const bar = 1;
export interface Baz {}
export default function Qux() {}
export type T = string;
  export const nested = 2;
"""


def write(name, text):
    p = pathlib.Path(tempfile.mkdtemp(), name)
    p.write_text(text)
    return str(p)


class OutlineTest(unittest.TestCase):
    def test_go_outline(self):
        out = narrow_reads.outline(write("a.go", GO), "/")
        self.assertEqual([l.split(":", 1)[0] for l in out], ["5", "9", "13", "15", "19", "21"])
        self.assertIn("func (q *Queue) Push() {", out[1])
        self.assertIn("type Queue struct {", out[0])
        self.assertFalse(any("Println" in l for l in out))

    def test_ts_outline_exported_only(self):
        out = narrow_reads.outline(write("a.ts", TS), "/")
        self.assertEqual([l.split(":", 1)[0] for l in out], ["2", "4", "5", "6", "7"])

    def test_outline_capped(self):
        text = "".join(f"func f{i}() {{}}\n" for i in range(200))
        out = narrow_reads.outline(write("a.go", text), "/")
        self.assertEqual(len(out), narrow_reads.OUTLINE_CAP + 1)
        self.assertIn("more", out[-1])

    def test_saved_output_has_no_outline(self):
        self.assertEqual(narrow_reads.outline("/x/tool-results/a.txt", "/"), [])


class HookTest(unittest.TestCase):
    def run_hook(self, event):
        r = subprocess.run([sys.executable, "-I", str(HOOK)], input=json.dumps(event),
                           capture_output=True, text=True, check=True)
        return json.loads(r.stdout) if r.stdout else None

    def long_file(self):
        return write("big.go", "package x\n\nfunc Big() {}\n" + "// c\n" * 400)

    def test_read_refusal_includes_outline(self):
        out = self.run_hook({"tool_name": "Read", "tool_input": {"file_path": self.long_file()}})
        self.assertIn("3:func Big() {}", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_cat_refusal_includes_outline(self):
        out = self.run_hook({"tool_name": "Bash", "tool_input": {"command": f"cat {self.long_file()}"}})
        self.assertIn("3:func Big() {}", out["hookSpecificOutput"]["permissionDecisionReason"])

    def test_short_file_allowed(self):
        self.assertIsNone(self.run_hook({"tool_name": "Read", "tool_input": {"file_path": write("s.go", "package x\n")}}))


if __name__ == "__main__":
    unittest.main()
