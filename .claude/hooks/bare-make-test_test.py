"""Tests for bare-make-test.py. Run: python3 -I -m unittest discover -s .claude/hooks -p '*_test.py'"""
import importlib.util
import pathlib
import unittest

_spec = importlib.util.spec_from_file_location("bare_make_test", pathlib.Path(__file__).with_name("bare-make-test.py"))
bare_make_test = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(bare_make_test)
piped = bare_make_test.piped_make_test
full = bare_make_test.full_suite_command


class BareMakeTestTest(unittest.TestCase):
    def test_piped(self):
        self.assertTrue(piped("make test | tail -20"))
        self.assertTrue(piped("make test 2>&1 | tail -20"))
        self.assertTrue(piped("make test-verbose |& grep FAIL"))
        self.assertTrue(piped("make -j4 test | cat"))
        self.assertTrue(piped("make test|tail"))

    def test_chained(self):
        self.assertTrue(piped("make lint 2>&1 | tail -8; make test 2>&1 | tail -20"))
        self.assertTrue(piped("make typecheck && make test | tail"))
        self.assertTrue(piped("make lint\nmake test | tail"))
        self.assertTrue(piped("cd x && true | make test | cat"))

    def test_bare_passes(self):
        self.assertFalse(piped("make test"))
        self.assertFalse(piped("make test-verbose"))
        self.assertFalse(piped("make test 2>&1"))
        self.assertFalse(piped("make test > out.log 2>&1"))
        self.assertFalse(piped("make test && echo ok"))
        self.assertFalse(piped("make test || tail log"))
        self.assertFalse(piped("make test; tail log"))
        self.assertFalse(piped("make test &\ntail log"))

    def test_other_pipes_pass(self):
        self.assertFalse(piped("make lint 2>&1 | tail -8"))
        self.assertFalse(piped("make vitest F=a.ts | tail"))
        self.assertFalse(piped("make typecheck | tail; make test"))
        self.assertFalse(piped("git log | grep test"))
        self.assertFalse(piped("echo make test | cat"))
        self.assertFalse(piped("echo 'make test | tail'"))

    def test_full_suite_refused(self):
        self.assertTrue(full("make vitest"))
        self.assertTrue(full("make vitest 2>&1 | tail -6"))
        self.assertTrue(full("make wiretypes; make vitest"))
        self.assertTrue(full("go test ./..."))
        self.assertTrue(full("go test -count=1 ./..."))
        self.assertTrue(full("make vitest 2>&1 | tail -6; go test ./... | grep -v ^ok"))

    def test_partial_suite_allowed(self):
        self.assertFalse(full("make vitest F=web/src/x.test.ts"))
        self.assertFalse(full("make vitest-related F=web/src/x.ts"))
        self.assertFalse(full("make vitest-stress F=web/src/x.test.ts"))
        self.assertFalse(full("go test ./core"))
        self.assertFalse(full("go test -count=1 ./core ./server"))
        self.assertFalse(full("make test"))
        self.assertFalse(full("echo make vitest"))
        self.assertFalse(full("git commit -m 'go test ./...'"))


if __name__ == "__main__":
    unittest.main()
