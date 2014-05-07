import os

from setuptools import setup, find_packages


here = os.path.abspath(os.path.dirname(__file__))
README = open(os.path.join(here, 'README.rst')).read()
CHANGES = open(os.path.join(here, 'CHANGES.rst')).read()


requires = ['reg',
            'morepath',
            'more.zodb',
            'more.transaction',
            'rdflib',
            'rdflib_zodb']


tests_require = ['nose',
                 'coverage']


setup(name='sprezz',
      version='0.2a0',
      description='sprezz',
      long_description=README + '\n\n' + CHANGES,
      classifiers=["Programming Language :: Python :: 3.3",
                   "Framework :: Morepath",
                   "Topic :: Internet :: WWW/HTTP",
                   "Topic :: Internet :: WWW/HTTP :: WSGI :: Application"],
      author='Olaf Conradi',
      author_email='olaf@conradi.org',
      url='http://sprezz.net/',
      packages=find_packages(),
      include_package_data=True,
      zip_safe=False,
      install_requires=requires,
      tests_require=tests_require,
      test_suite='sprezz',
      entry_points={
          'console_scripts': [
              'sprezz = sprezz:main',
              ]
          },
      )
