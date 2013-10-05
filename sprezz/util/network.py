import logging

from requests import codes, Session
from requests.compat import urljoin, urlparse
from requests.cookies import extract_cookies_to_jar
from requests.exceptions import (
    ConnectionError,
    SSLError,
    RequestException,
    TooManyRedirects,
    Timeout,
    HTTPError,
)
from requests.utils import requote_uri


log = logging.getLogger(__name__)


TIMEOUT = 3.0


_extra_codes = {
    308: ('permanent_redirect', 'redirect'),
}


# Add extra codes to ``codes`` LookupDict
for (code, titles) in list(_extra_codes.items()):
    for title in titles:
        setattr(codes, title, code)
        if not title.startswith('\\'):
            setattr(codes, title.upper(), code)


REDIRECT_STATI = (
    codes.moved,  # 301
    codes.found,  # 302
    codes.other,  # 303
    codes.temporary_moved,  # 307
    codes.permanent_redirect,  # 308
)


class StrictSession(Session):
    def resolve_redirects(self, resp, req, stream=False, timeout=None,
                          verify=True, cert=None, proxies=None):
        """Receives a Response. Returns a generator of Responses.

        :class:`StrictSession` does not force the method to GET on a 301
        moved permanently Response. It will leave the method untouched unless a
        303 See Other response is returned. In that case a GET method is set.
        """
        i = 0

        # ((resp.status_code is codes.see_other))
        while (('location' in resp.headers) and (
               resp.status_code in REDIRECT_STATI)):
            prepared_request = req.copy()

            resp.content  # Consume socket so it can be released

            if i >= self.max_redirects:
                raise TooManyRedirects('Exceeded %s redirects.' % (
                                       self.max_redirects))

            # Release the connection back into the pool.
            resp.close()

            url = resp.headers['location']
            method = req.method

            # Handle redirection without scheme (see: RFC 1808 Section 4)
            if url.startswith('//'):
                parsed_rurl = urlparse(resp.url)
                url = '%s:%s' % (parsed_rurl.scheme, url)

            # The scheme should be lower case...
            if '://' in url:
                scheme, uri = url.split('://', 1)
                url = '%s://%s' % (scheme.lower(), uri)

            # Facilitate non-RFC2616-compliant 'location' headers
            # (e.g. '/path/to/resource' instead of
            # 'http://domain.tld/path/to/resource')
            # Compliant with RFC3986, we percent encode the url.
            if not urlparse(url).netloc:
                url = urljoin(resp.url, requote_uri(url))
            else:
                url = requote_uri(url)

            prepared_request.url = url

            # http://www.w3.org/Protocols/rfc2616/rfc2616-sec10.html#sec10.3.4
            if (resp.status_code == codes.see_other and
                    method != 'HEAD'):
                method = 'GET'

            # Don't do what the browsers do, follow standards by
            # making a comment of forcing 301 redirect to GET.
            # if (resp.status_code in (codes.moved, codes.found) and
            #         method not in ('GET', 'HEAD')):
            #     method = 'GET'

            prepared_request.method = method

            # https://github.com/kennethreitz/requests/issues/1084
            # and extend this list to all redirect codes that should
            # keep their body.
            if resp.status_code not in (codes.moved,
                                        codes.found,
                                        codes.temporary,
                                        codes.permanent_redirect):
                if 'Content-Length' in prepared_request.headers:
                    del prepared_request.headers['Content-Length']
                prepared_request.body = None

            headers = prepared_request.headers
            try:
                del headers['Cookie']
            except KeyError:
                pass

            prepared_request.prepare_cookies(self.cookies)

            resp = self.send(
                prepared_request,
                stream=stream,
                timeout=timeout,
                verify=verify,
                cert=cert,
                proxies=proxies,
                allow_redirects=False,
            )

            extract_cookies_to_jar(self.cookies, prepared_request, resp.raw)

            i += 1
            yield resp


def session():
    """Returns a :class:`StrictSession` for context-management."""
    return StrictSession()


def request(method, url, **kwargs):
    kwargs.setdefault('allow_redirects', True)
    kwargs.setdefault('verify', True)
    kwargs.setdefault('timeout', TIMEOUT)
    session = StrictSession()
    response = session.request(method=method, url=url, **kwargs)
    # Raise an exception when 400 <= status codes < 600
    response.raise_for_status()
    return response


def get(url, **kwargs):
    return request('GET', url, **kwargs)


def options(url, **kwargs):
    return request('OPTIONS', url, **kwargs)


def head(url, **kwargs):
    return request('HEAD', url, **kwargs)


def post(url, data=None, **kwargs):
    return request('POST', url, data=data, **kwargs)


def put(url, data=None, **kwargs):
    return request('PUT', url, data=data, **kwargs)


def patch(url, data=None, **kwargs):
    return request('PATCH', url, data=data, **kwargs)
