import unittest

from pyramid import testing

from ..testing import create_single_content_registry


class TestWellKnown(unittest.TestCase):
    def setUp(self):
        self.config = testing.setUp()

    def tearDown(self):
        testing.tearDown()

    def _makeOne(self):
        from sprezz.wellknown import WellKnown
        return WellKnown()

    def test_after_create(self):
        inst = self._makeOne()
        Z = {}
        def add(name, val, registry=None):
            Z[name] = val
        inst.add = add
        ob = testing.DummyResource()
        registry = create_single_content_registry(ob)
        inst.after_create(None, registry)
        self.assertEqual(Z['zot-info'], ob)
