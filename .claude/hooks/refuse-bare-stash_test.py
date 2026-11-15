"""Tests for refuse-bare-stash.py. Run: python3 -I -m unittest discover -s .claude/hooks -p '*_test.py'"""
import importlib.util
import pathlib
import unittest

_spec = importlib.util.spec_from_file_location("refuse_bare_stash", pathlib.Path(__file__).with_name("refuse-bare-stash.py"))
refuse_bare_stash = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(refuse_bare_stash)
unsafe_stash = refuse_bare_stash.unsafe_stash


class RefuseBareStashTest(unittest.TestCase):
    def test_bare_stash(self):
        self.assertTrue(unsafe_stash("git stash"))
        self.assertTrue(unsafe_stash("git stash -u"))
        self.assertTrue(unsafe_stash("git stash -q"))

    def test_pop(self):
        self.assertTrue(unsafe_stash("git stash pop"))
        self.assertTrue(unsafe_stash("git stash pop -q"))
        self.assertTrue(unsafe_stash("git stash pop stash@{1}"))

    def test_push_without_message(self):
        self.assertTrue(unsafe_stash("git stash push"))
        self.assertTrue(unsafe_stash("git stash push -u -q"))
        self.assertTrue(unsafe_stash("git stash push -- a.go"))

    def test_push_with_message_passes(self):
        self.assertFalse(unsafe_stash("git stash push -m s704"))
        self.assertFalse(unsafe_stash("git stash push -u -m s704 -q"))
        self.assertFalse(unsafe_stash("git stash push -um s704"))
        self.assertFalse(unsafe_stash("git stash push --message=s704"))
        self.assertFalse(unsafe_stash("git stash push --message s704"))

    def test_other_subcommands_pass(self):
        for sub in ("list", "show", "apply abc123", "drop stash@{0}", "branch b"):
            self.assertFalse(unsafe_stash("git stash " + sub))

    def test_chained_commands(self):
        self.assertTrue(unsafe_stash("git stash push -u -m s704 -q; make test; git stash pop -q"))
        self.assertTrue(unsafe_stash("make test && git stash"))
        self.assertTrue(unsafe_stash("git diff | cat || git stash pop"))
        self.assertTrue(unsafe_stash("make test\ngit stash pop"))
        self.assertFalse(unsafe_stash("git stash push -m x; git stash apply abc; git stash list | head"))

    def test_cd_before_git(self):
        self.assertTrue(unsafe_stash("cd /tmp/repo && git stash pop"))
        self.assertTrue(unsafe_stash("cd /tmp/repo; git stash"))
        self.assertFalse(unsafe_stash("cd /tmp/repo && git stash push -m x"))

    def test_git_global_options(self):
        self.assertTrue(unsafe_stash("git -C /tmp/repo stash pop"))
        self.assertTrue(unsafe_stash("git -c core.x=1 stash"))
        self.assertFalse(unsafe_stash("git -C /tmp/repo stash list"))

    def test_other_commands_pass(self):
        self.assertFalse(unsafe_stash("git status"))
        self.assertFalse(unsafe_stash("echo git stash pop"))
        self.assertFalse(unsafe_stash("grep 'git stash' docs/x.md"))
        self.assertFalse(unsafe_stash("git commit -m 'stash pop'"))


if __name__ == "__main__":
    unittest.main()
