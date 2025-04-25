{ lib, stdenv, fetchurl, python311, writeText, termcap}:

let
  Modules_Setup_local = writeText "Modules_Setup_local" ''
    _asyncio _asynciomodule.c
    _bisect _bisectmodule.c
    _contextvars _contextvarsmodule.c
    _csv _csv.c
    _datetime _datetimemodule.c
    _decimal _decimal/_decimal.c
    _heapq _heapqmodule.c
    _json _json.c
    _lsprof _lsprof.c rotatingtree.c
    _multiprocessing -I$(srcdir)/Modules/_multiprocessing _multiprocessing/multiprocessing.c _multiprocessing/semaphore.c
    _opcode _opcode.c
    _pickle _pickle.c
    _queue _queuemodule.c
    _random _randommodule.c
    _socket socketmodule.c
    _statistics _statisticsmodule.c
    _struct _struct.c
    _typing _typingmodule.c
    _zoneinfo _zoneinfo.c
    array arraymodule.c
    audioop audioop.c
    binascii binascii.c
    cmath cmathmodule.c
    math mathmodule.c
    mmap mmapmodule.c
    select selectmodule.c

    _blake2 _blake2/blake2module.c _blake2/blake2b_impl.c _blake2/blake2s_impl.c
    _md5 md5module.c
    _sha1 sha1module.c
    _sha256 sha256module.c
    _sha512 sha512module.c
    _sha3 _sha3/sha3module.c
    unicodedata unicodedata.c

    _posixsubprocess _posixsubprocess.c
    _posixshmem -I$(srcdir)/Modules/_multiprocessing _multiprocessing/posixshmem.c -lrt
    fcntl fcntlmodule.c
    grp grpmodule.c
    ossaudiodev ossaudiodev.c
    resource resource.c
    spwd spwdmodule.c
    syslog syslogmodule.c
    termios termios.c

    _elementtree _elementtree.c
    pyexpat pyexpat.c

    _bz2 _bz2module.c -lbz2
    _ctypes _ctypes/_ctypes.c _ctypes/callbacks.c _ctypes/callproc.c _ctypes/stgdict.c _ctypes/cfield.c -ldl -lffi -DHAVE_FFI_PREP_CIF_VAR -DHAVE_FFI_PREP_CLOSURE_LOC -DHAVE_FFI_CLOSURE_ALLOC
    _dbm _dbmmodule.c -lgdbm_compat -DUSE_GDBM_COMPAT
    _gdbm _gdbmmodule.c -lgdbm
    _lzma _lzmamodule.c -llzma
    #_uuid _uuidmodule.c -luuid
    zlib  zlibmodule.c -lz

    _ssl _ssl.c $(OPENSSL_INCLUDES) $(OPENSSL_LDFLAGS) $(OPENSSL_LIBS)
    _hashlib _hashopenssl.c $(OPENSSL_INCLUDES) $(OPENSSL_LDFLAGS) -lcrypto

      _ssl _ssl.c $(OPENSSL_INCLUDES) $(OPENSSL_LDFLAGS) \
          -l:libssl.a -Wl,--exclude-libs,libssl.a \
          -l:libcrypto.a -Wl,--exclude-libs,libcrypto.a
      _hashlib _hashopenssl.c $(OPENSSL_INCLUDES) $(OPENSSL_LDFLAGS) \
          -l:libcrypto.a -Wl,--exclude-libs,libcrypto.a

    _curses -lncurses -lncursesw -ltermcap _cursesmodule.c
    _curses_panel -lpanel -lncurses _curses_panel.c
  '';
in
python311.overrideAttrs (oldAttrs: rec {
    # https://wiki.python.org/moin/BuildStatically
    # https://github.com/python/cpython/blob/3.11/Modules/Setup
    configureFlags = oldAttrs.configureFlags ++ [
      "LDFLAGS=-L${termcap}/lib"
      #"--with-ensurepip=install"
    ];
    stripIdlelib = true;
    stripTests = true;
    stripTkinter = true;
    postPatch = oldAttrs.postPatch + ''
      cp ${Modules_Setup_local} Modules/Setup.local
    '';
})
