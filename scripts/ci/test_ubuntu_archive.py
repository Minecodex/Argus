import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch
import ubuntu_archive as subject

class UbuntuArchiveTests(unittest.TestCase):
    def test_healthy_primary_keeps_sources_unchanged(self):
        files={subject.SOURCES[0]:"deb http://archive.ubuntu.com/ubuntu noble main\n"}
        calls=[]
        self.assertEqual(subject.prepare(files,"noble",lambda base,code:calls.append(base) or True),{})
        self.assertEqual(calls,[subject.PRIMARY])

    def test_fallback_preserves_signature_and_other_sources(self):
        original="Types: deb\nURIs: http://azure.archive.ubuntu.com/ubuntu\nSuites: noble noble-updates noble-security\nComponents: main universe\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n"
        other=Path("/etc/apt/sources.list.d/microsoft-prod.list")
        files={subject.SOURCES[1]:original,other:"deb https://packages.microsoft.com/ubuntu/24.04/prod noble main\n"}
        changes=subject.prepare(files,"noble",lambda base,code:base==subject.MIRRORS[0])
        self.assertEqual(set(changes),{subject.SOURCES[1]})
        self.assertIn("Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg",changes[subject.SOURCES[1]])
        self.assertIn("Suites: noble noble-updates noble-security",changes[subject.SOURCES[1]])
        self.assertNotIn("trusted=yes",changes[subject.SOURCES[1]])

    def test_hosted_mirror_file_preserves_signed_source(self):
        original="URIs: mirror+file:/etc/apt/apt-mirrors.txt\nSigned-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg\n"
        self.assertEqual(subject.plan_sources({subject.SOURCES[1]:original},subject.MIRRORS[0]),{subject.MIRROR_FILE:subject.MIRRORS[0]+"\n"})

    def test_legacy_security_uri_uses_signed_archive(self):
        text="deb https://security.ubuntu.com/ubuntu/ noble-security main\n"
        self.assertEqual(subject.plan_sources({subject.SOURCES[0]:text},subject.MIRRORS[0])[subject.SOURCES[0]],"deb "+subject.MIRRORS[0]+" noble-security main\n")

    def test_unreachable_mirrors_fail_closed(self):
        with self.assertRaises(RuntimeError):
            subject.prepare({},"noble",lambda *_:False)

    def test_unknown_configuration_and_unapproved_inputs_fail_closed(self):
        with self.assertRaises(RuntimeError):
            subject.plan_sources({subject.SOURCES[0]:"deb https://custom.invalid/repo noble main"},subject.MIRRORS[0])
        with self.assertRaises(ValueError):
            subject.plan_sources({},"http://unapproved.invalid")
        with self.assertRaises(ValueError):
            subject.prepare({},"../../other",lambda *_:True)

    def test_probe_checks_release_and_timeout(self):
        body=b"-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA512\n\nOrigin: Ubuntu\nCodename: noble\n"
        self.assertTrue(subject.release_valid(body,"noble"))
        self.assertFalse(subject.release_valid(body,"resolute"))
        self.assertFalse(subject.release_valid(b"proxy returned HTML","noble"))
        with patch.object(subject.subprocess,"run",return_value=subprocess.CompletedProcess([],0,b"invalid",b"")):
            self.assertFalse(subject.probe(subject.MIRRORS[0],"noble"))
        with patch.object(subject.subprocess,"run",side_effect=subprocess.TimeoutExpired("curl",20)):
            self.assertFalse(subject.probe(subject.MIRRORS[0],"noble"))

if __name__ == "__main__":
    unittest.main()
