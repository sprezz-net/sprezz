import morepath
import pkg_resources

from more.transaction import transaction_app
from more.zodb import zodb_app


version = pkg_resources.get_distribution('sprezz').version


app = morepath.App(name='Sprezz', extends=[zodb_app, transaction_app])


@app.path(path='')
class Root(object):
    pass


@app.view(model=Root)
def hello_world(self, request):
    return 'Hello World!'


def main():
    morepath.autosetup()
    # XXX For testing purposes bind to all interfaces
    morepath.run(app, host='0.0.0.0')


if __name__ == '__main__':
    main()
