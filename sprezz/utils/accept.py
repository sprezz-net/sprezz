from logging import getLogger

from aiohttp import hdrs
from aiohttp.web import HTTPMethodNotAllowed, HTTPNotAcceptable
from aiohttp.helpers import parse_mimetype


log = getLogger(__name__)


def parse_accept(accept):
    parts = accept.split(',')
    types = []
    for item in parts:
        if not item:
            continue
        mimetype = parse_mimetype(item)
        mimestring = '+'.join(filter(None,
                                     ['/'.join(filter(None,
                                                      [mimetype.type,
                                                       mimetype.subtype])),
                                      mimetype.suffix]))
        types.append(mimestring)
    return types


class AcceptChooser:
    def __init__(self):
        self._accepts = {}

    def accept(self, method, accept):
        def inner(handler):
            if method not in self._accepts:
                self._accepts[method] = {}
            for mtype in parse_accept(accept):
                self._accepts[method][mtype] = handler
            return handler
        return inner

    async def route(self, request):
        if request.method not in self._accepts:
            self._raise_allowed_methods(request)
        for accept in request.headers.getall(hdrs.ACCEPT, []):
            for mimetype in parse_accept(accept):
                acceptor = self._accepts[request.method].get(mimetype)
                if acceptor is not None:
                    log.debug('Choosing route for content type %s', mimetype)
                    resp = await acceptor(request)
                    return resp
        raise HTTPNotAcceptable()

    def _raise_allowed_methods(self, request):
        allowed_methods = {m for m in self._accepts}
        raise HTTPMethodNotAllowed(request.method, allowed_methods)
