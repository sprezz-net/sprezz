#!/usr/bin/env python3
"""Sprezz setup script."""
import os
import sys

from datetime import date
from setuptools import find_packages, setup
from setuptools.command.test import test as TestCommand


PROJECT_NAME = 'Sprezz'
PROJECT_PACKAGE_NAME = 'sprezz'
PROJECT_LICENSE = 'Apache License 2.0'
PROJECT_AUTHOR = 'Olaf Conradi'
PROJECT_COPYRIGHT = '2013-{}, {}'.format(date.today().year, PROJECT_AUTHOR)
PROJECT_URL = 'https://sprezz.net/'
PROJECT_EMAIL = 'hello@sprezz.net'
PROJECT_DESCRIPTION = 'Sprezz is a federated social network.'
PROJECT_CLASSIFIERS = [
    'Development Status :: 2 - Pre-Alpha',
    'Framework :: AsyncIO',
    'License :: OSI Approved :: Apache Software License',
    'Operating System :: POSIX',
    'Programming Language :: Python :: 3.6',
    'Topic :: Communications',
    'Topic :: Internet :: WWW/HTTP']
PROJECT_REQUIRES = [
    'aiohttp>=3.1.0',
    'aiohttp_remotes',
    'aiofiles',
    'asyncpg',
    'gino>=0.6.2',
    'passlib[bcrypt]',
    'trafaret-config',
    'trafaret_validator']
PROJECT_PACKAGES = find_packages(exclude=('tests',))
PROJECT_TEST_REQUIRES = [
    'pytest']


HERE = os.path.abspath(os.path.dirname(__file__))
with open(os.path.join(HERE, 'README.rst'), encoding='utf-8') as f:
    README = '\n' + f.read()
with open(os.path.join(HERE, 'CHANGES.rst'), encoding='utf-8') as f:
    CHANGES = '\n' + f.read()


ABOUT = {}
with open(os.path.join(HERE, PROJECT_PACKAGE_NAME, '__version__.py')) as f:
    exec(f.read(), ABOUT)


class PyTest(TestCommand):
    user_options = []

    def run(self):
        import subprocess
        errno = subprocess.call([sys.executable, '-m', 'pytest', 'tests'])
        raise SystemExit(errno)


setup(
    name=PROJECT_PACKAGE_NAME,
    version=ABOUT['__version__'],
    license=PROJECT_LICENSE,
    url=PROJECT_URL,
    description=PROJECT_DESCRIPTION,
    long_description=README + '\n' + CHANGES,
    author=PROJECT_AUTHOR,
    author_email=PROJECT_EMAIL,
    packages=PROJECT_PACKAGES,
    platforms=['POSIX'],
    classifiers=PROJECT_CLASSIFIERS,
    install_requires=PROJECT_REQUIRES,
    include_package_data=True,
    test_suite='tests',
    tests_require=PROJECT_TEST_REQUIRES,
    entry_points={
        'console_scripts': [
            'sprezz = sprezz.__main__:main'
        ]
    },
    extras_require={
        'testing': PROJECT_TEST_REQUIRES,
    },
    cmdclass={
        'test': PyTest
    },
)
