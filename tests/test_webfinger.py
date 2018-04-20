import os
import ssl

import pytest

from aiohttp import web

from sprezz.views.webfinger import Resource, setup_routes


def test_resource():
    base = {'scheme': 'acct',
            'username': 'carol',
            'host': 'example.com',
            'port': None,
            'path': '',
            'query': '',
            'fragment': ''}

    res = Resource("acct:carol@example.com")
    assert res.as_dict() == base

    res = Resource("carol@example.com")
    assert res.as_dict() == base

    res = Resource("https://carol@example.com")
    assert res.as_dict() == {**base, 'scheme': 'https'}

    res = Resource("carol@example.com:8080")
    assert res.as_dict() == {**base, 'scheme': 'https', 'port': 8080}

    res = Resource("https://carol@example.com:8080")
    assert res.as_dict() == {**base, 'scheme': 'https', 'port': 8080}

    res = Resource("example.com")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None}

    res = Resource("https://example.com")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None}

    res = Resource("example.com/joe")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe'}

    res = Resource("https://example.com/joe")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe'}

    res = Resource("example.com/joe?p1=one&p2=two")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'query': 'p1=one&p2=two'}

    res = Resource("https://example.com/joe?p1=one&p2=two")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'query': 'p1=one&p2=two'}

    res = Resource("https://example.com/joe#frag")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'fragment': 'frag'}

    res = Resource("example.com/joe?p1=one&p2=two#f1/f2")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'query': 'p1=one&p2=two',
                             'fragment': 'f1/f2'}

    res = Resource("https://example.com/joe?p1=one&p2=two#f1/f2")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'query': 'p1=one&p2=two',
                             'fragment': 'f1/f2'}

    res = Resource("https://example.com/joe/../bob?p1=one&p2=two")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/bob',
                             'query': 'p1=one&p2=two'}

    res = Resource("https://example.com/joe#f1/../f2")
    assert res.as_dict() == {**base, 'scheme': 'https', 'username': None,
                             'path': '/joe',
                             'fragment': 'f2'}


def webfinger_app(loop):
    app = web.Application()
    setup_routes(app)
    return app


async def test_webfinger_http(aiohttp_client):
    client = await aiohttp_client(webfinger_app)
    params = {'resource': 'carol@example.com'}
    headers = {'accept': 'application/jrd+json'}
    # Test without accept content-type
    resp = await client.get('/.well-known/webfinger',
                            params=params)
    assert resp.status == 406
    # Test WebFinger without HTTPS
    resp = await client.get('/.well-known/webfinger',
                            params=params,
                            headers=headers)
    assert resp.status == 500
    assert resp.reason == 'WebFinger requires HTTPS'


async def test_webfinger_https(aiohttp_client):
    here = os.path.abspath(os.path.dirname(__file__))
    # ssl_context = ssl.create_default_context(cafile=here + '/localhost.crt')
    ssl_context = ssl.SSLContext(ssl.PROTOCOL_SSLv23)
    ssl_context.check_hostname = False
    ssl_context.verify_mode = ssl.CERT_NONE
    ssl_context.load_cert_chain(certfile=here + '/localhost.crt',
                                keyfile=here + '/localhost.key')
    client = await aiohttp_client(webfinger_app,
                                  server_kwargs={'scheme': 'https',
                                                 'ssl': ssl_context})
    params = {'resource': 'carol@example.com'}
    headers = {'accept': 'application/jrd+json'}
    # Test without accept content-type
    resp = await client.get('/.well-known/webfinger',
                            params=params,
                            ssl=ssl_context)
    assert resp.status == 406
    # Test with accept content-type
    resp = await client.get('/.well-known/webfinger',
                            params=params,
                            headers=headers,
                            scheme='https',
                            ssl=ssl_context)
    assert resp.status == 200
