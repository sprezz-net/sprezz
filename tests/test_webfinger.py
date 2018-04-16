from sprezz.views.webfinger import Resource


class TestWebFinger:
    def test_resource(self):
        base = {'scheme': 'acct',
                'username': 'carol',
                'host': 'example.com',
                'port': None,
                'path': '',
                'query': '',
                'fragment': ''}

        res = Resource("acct:carol@example.com")
        assert res.asdict() == base

        res = Resource("carol@example.com")
        assert res.asdict() == base

        res = Resource("https://carol@example.com")
        assert res.asdict() == {**base, 'scheme': 'https'}

        res = Resource("carol@example.com:8080")
        assert res.asdict() == {**base, 'scheme': 'https', 'port': 8080}

        res = Resource("https://carol@example.com:8080")
        assert res.asdict() == {**base, 'scheme': 'https', 'port': 8080}

        res = Resource("example.com")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None}

        res = Resource("https://example.com")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None}

        res = Resource("example.com/joe")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe'}

        res = Resource("https://example.com/joe")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe'}

        res = Resource("example.com/joe?p1=one&p2=two")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe',
                                'query': 'p1=one&p2=two'}

        res = Resource("https://example.com/joe?p1=one&p2=two")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe',
                                'query': 'p1=one&p2=two'}

        res = Resource("example.com/joe?p1=one&p2=two#f1/f2")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe',
                                'query': 'p1=one&p2=two',
                                'fragment': 'f1/f2'}

        res = Resource("https://example.com/joe?p1=one&p2=two#f1/f2")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/joe',
                                'query': 'p1=one&p2=two',
                                'fragment': 'f1/f2'}

        res = Resource("https://example.com/joe/../bob?p1=one&p2=two")
        assert res.asdict() == {**base, 'scheme': 'https', 'username': None,
                                'path': '/bob',
                                'query': 'p1=one&p2=two'}
