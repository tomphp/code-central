"""Tests for prefer-edit.py. Run: python3 -I -m unittest discover -s .claude/hooks -p '*_test.py'"""
import importlib.util
import pathlib
import unittest

_spec = importlib.util.spec_from_file_location("prefer_edit", pathlib.Path(__file__).with_name("prefer-edit.py"))
prefer_edit = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(prefer_edit)
edits_one_file = prefer_edit.edits_one_file


class PreferEditTest(unittest.TestCase):
    def test_glossary_edit_with_bold_text(self):
        cmd = """python3 - <<'EOF'
p = 'GLOSSARY.md'
s = open(p).read()
s = s.replace('**Implement**', '**Build**')
open(p, 'w').write(s)
EOF"""
        self.assertTrue(edits_one_file(cmd))

    def test_python_edit_with_for_loop(self):
        cmd = """python3 <<EOF
from pathlib import Path
p = Path('internal/a.go')
text = p.read_text()
for old in ['a', 'b']:
    text = text.replace(old, 'c')
p.write_text(text)
EOF"""
        self.assertTrue(edits_one_file(cmd))

    def test_heredoc_findall_tokens_do_not_count(self):
        cmd = """grep -n x a.go b.go c.go; python3 - <<"END"
s = open('x.md').read().replace('a.go b.go c.go', 'd')
open('x.md', 'w').write(s)
END"""
        self.assertTrue(edits_one_file(cmd))

    def test_sed_one_file_after_grep_of_others(self):
        self.assertTrue(edits_one_file("grep -n foo a.go b.go c.go && sed -i 's/foo/bar/' internal/x.go"))

    def test_sed_several_files(self):
        self.assertFalse(edits_one_file("sed -i 's/foo/bar/' a.go b.go"))

    def test_multi_file_rename_passes(self):
        self.assertFalse(edits_one_file("for f in a.go b.go; do sed -i 's/x/y/' $f; done"))
        self.assertFalse(edits_one_file("find . -name '*.go' | xargs sed -i 's/x/y/'"))
        self.assertFalse(edits_one_file("sed -i 's/x/y/' *.go"))
        cmd = """python3 - <<'EOF'
for p in ['a.go', 'b.go']:
    s = open(p).read().replace('x', 'y')
    open(p + '.new', 'w').write(s)
EOF"""
        self.assertFalse(edits_one_file(cmd))

    def test_perl_in_place_one_file(self):
        self.assertTrue(edits_one_file("perl -0pi -e 's/foo/bar/' web/src/App.tsx"))
        self.assertTrue(edits_one_file("perl -pi -e 's/foo/bar/' internal/x.go"))
        self.assertTrue(edits_one_file("perl -i.bak -pe 's/foo/bar/' README.md"))
        self.assertTrue(edits_one_file("grep -n foo a.go b.go && perl -i -pe 's/foo/bar/' c.go"))

    def test_perl_in_place_several_files(self):
        self.assertFalse(edits_one_file("perl -pi -e 's/foo/bar/' a.go b.go"))
        self.assertFalse(edits_one_file("perl -pi -e 's/foo/bar/' *.go"))
        self.assertFalse(edits_one_file("find . -name '*.go' | xargs perl -pi -e 's/x/y/'"))
        self.assertFalse(edits_one_file("for f in a.go b.go; do perl -pi -e 's/x/y/' $f; done"))

    def test_perl_without_in_place_passes(self):
        self.assertFalse(edits_one_file("perl -ne 'print if /foo/' a.go"))
        self.assertFalse(edits_one_file("perl -pe 's/foo/bar/' a.go > /tmp/out"))

    def test_perl_script_rewriting_one_path(self):
        cmd = """perl <<'EOF'
my $p = 'GLOSSARY.md';
open(my $in, '<', $p) or die;
local $/; my $s = <$in>; close $in;
$s =~ s/Implement/Build/g;
open(my $out, '>', $p) or die;
print $out $s;
EOF"""
        self.assertTrue(edits_one_file(cmd))
        cmd = """cat > /tmp/docedit.pl <<'EOF'
open(F, "<$file"); my $s = join('', <F>); close F;
open(F, ">$file"); print F $s;
EOF
perl /tmp/docedit.pl"""
        self.assertTrue(edits_one_file(cmd))

    def test_perl_script_reading_one_path_and_writing_another_passes(self):
        cmd = """perl <<'EOF'
open(my $in, '<', $a) or die;
open(my $out, '>', $b) or die;
EOF"""
        self.assertFalse(edits_one_file(cmd))

    def test_read_only_commands_pass(self):
        self.assertFalse(edits_one_file("go test ./..."))
        self.assertFalse(edits_one_file("python3 - <<'EOF'\nprint(open('a.md').read().replace('a','b'))\nEOF"))


if __name__ == "__main__":
    unittest.main()
