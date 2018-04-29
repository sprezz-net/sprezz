import logging
import venusian

from aiohttp import hdrs


log = logging.getLogger(__name__)


class plugin:
    def __init__(self, method=None, path=None,
                 add_routes=False,
                 *args, **kwargs):
        self._method = method
        self._path = path
        self._add_routes = add_routes
        self._args = args
        self._kwargs = kwargs

    def __call__(self, func):
        def callback(scanner, name, ob):
            log.debug("name %s", name)
            if self._method is not None and self._path is not None:
                scanner.router.add_route(self._method, self._path, ob,
                                         *self._args, **self._kwargs)
            elif self._add_routes:
                ob(scanner.app)
        venusian.attach(func, callback)
        return func

    @classmethod
    def route(cls, method, path, *args, **kwargs):
        return cls(method, path, *args, **kwargs)

    @classmethod
    def get(cls, path, *args, **kwargs):
        return cls(hdrs.METH_GET, path, *args, **kwargs)

    @classmethod
    def add_routes(cls):
        return cls(add_routes=True)
