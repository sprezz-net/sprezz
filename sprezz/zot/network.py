from urllib3 import PoolManager


class ZotNetwork(object):

    def __init__(self):
        # TODO Support ProxyManager
        self.conn = PoolManager()

    def fetch(self, url):
        """Fetch data from url.

        Equivalent of z_fetch_url.

        """
        r = self.conn.request('GET', url, retries=8, redirect=True)
        return r

    def post(url, fields):
        """Post data to url.

        Equivalent of z_post_url.

        """
        r = self.conn.request('POST', url, fields, retries=8, redirect=True)
        return r
