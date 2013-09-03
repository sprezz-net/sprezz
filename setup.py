import os

from setuptools import setup, find_packages

here = os.path.abspath(os.path.dirname(__file__))
README = open(os.path.join(here, 'README.rst')).read()
CHANGES = open(os.path.join(here, 'CHANGES.rst')).read()

requires = [
    'pyramid>=1.5a1',
    'pyramid_zodbconn',
    'transaction',
    'pyramid_tm',
    'pyramid_debugtoolbar',
    'waitress',
    'ZODB>=4.0.0b3',
    'rdflib==4.1-dev',
    'rdflib_zodb',
    'urllib3',
    'whirlpool>0.3',
    'pycrypto>=2.6',
    'zope.copy',
    'zope.component',
    'venusian',
    ]

setup(name='sprezz',
      version='0.0',
      description='sprezz',
      long_description=README + '\n\n' + CHANGES,
      classifiers=[
        "Programming Language :: Python :: 3.3",
        "Framework :: Pyramid",
        "Topic :: Internet :: WWW/HTTP",
        "Topic :: Internet :: WWW/HTTP :: WSGI :: Application",
        ],
      author='Olaf Conradi',
      author_email='olaf@conradi.org',
      url='',
      keywords='web pylons pyramid',
      packages=find_packages(),
      include_package_data=True,
      zip_safe=False,
      install_requires=requires,
      tests_require=requires,
      test_suite="sprezz",
      entry_points="""\
      [paste.app_factory]
      main = sprezz:main
      """,
      )
