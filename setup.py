import os
import re

from setuptools import setup, find_packages


here = os.path.abspath(os.path.dirname(__file__))
README = open(os.path.join(here, 'README.rst')).read()
CHANGES = open(os.path.join(here, 'CHANGES.rst')).read()

fp_init = open(os.path.join(here, 'sprezz', '__init__.py'))
VERSION = re.compile(r".*__version__ = '(.*?)'",
                     re.S).match(fp_init.read()).group(1)
fp_init.close()


requires = ['pyramid_chameleon',
            'pyramid>=1.5a2',
            'pyramid_zodbconn',
            'transaction',
            'pyramid_tm',
            'pyramid_debugtoolbar>=1.0.8',
            'waitress',
            'ZODB>=4.0.0b3',
            'rdflib==4.1-dev',
            'rdflib_zodb',
            'requests>=2.0.0',
            'whirlpool>0.3',  # devel
            'pycrypto>=2.6',  # devel
            'zope.copy',
            'zope.component',
            'venusian']


tests_require = ['nose',
                 'coverage']


setup(name='sprezz',
      version=VERSION,
      description='sprezz',
      long_description=README + '\n\n' + CHANGES,
      classifiers=["Programming Language :: Python :: 3.3",
                   "Framework :: Pyramid",
                   "Topic :: Internet :: WWW/HTTP",
                   "Topic :: Internet :: WWW/HTTP :: WSGI :: Application"],
      author='Olaf Conradi',
      author_email='olaf@conradi.org',
      url='http://sprezz.net/',
      keywords='web pylons pyramid',
      packages=find_packages(),
      include_package_data=True,
      zip_safe=False,
      install_requires=requires,
      tests_require=tests_require,
      test_suite="sprezz",
      entry_points="""\
      [paste.app_factory]
      main = sprezz:main
      """,
      )
