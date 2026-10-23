# Hyphenation patterns

The patterns come from the hyph-utf8 package (https://github.com/hyphenation/tex-hyphen),
as of 2026-10-17, and are included unchanged. pdx reads the
text versions (`*.pat.txt` with patterns, `*.hyp.txt` with exceptions).

| File | Language | License |
|---|---|---|
| de-1996.pat.txt | German, reformed spelling (de-1996), version 2024-02-28 | MIT |
| en-us.pat.txt, en-us.hyp.txt | English, American spelling (en-us), version 2005-05-30 | free redistribution with notice, see below |

The header lines of the original files with copyright and license follow
unchanged.

## de-1996

```
 title: German Hyphenation Patterns (Reformed Orthography, 2006)

 notice: TeX-Trennmuster für die reformierte (2006) deutsche Rechtschreibung

 version: 2024-02-28

 authors:
   -
     name:    Deutschsprachige Trennmustermannschaft
     contact: trennmuster@dante.de

 copyright: Copyright (c) 2013-2024
            Stephan Hennig, Werner Lemberg, Günter Milde,
            Sander van Geloven, Georg Pfeiffer, Gisbert W. Selke,
            Tobias Wendorf, Keno Wehr

 licence:
     name: MIT
     url:  https://opensource.org/licenses/mit-license.php
     text: >
           Permission is hereby granted, free of charge, to any person
           obtaining a copy of this software and associated documentation
           files (the “Software”), to deal in the Software without
           restriction, including without limitation the rights to use,
           copy, modify, merge, publish, distribute, sublicense, and/or
           sell copies of the Software, and to permit persons to whom the
           Software is furnished to do so, subject to the following
           conditions:

           The above copyright notice and this permission notice shall be
           included in all copies or substantial portions of the Software.

           THE SOFTWARE IS PROVIDED “AS IS”, WITHOUT WARRANTY OF ANY KIND,
           EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
           OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
           NONINFRINGEMENT.  IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
           HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
           WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
           FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
           OTHER DEALINGS IN THE SOFTWARE.

 source: https://repo.or.cz/w/wortliste.git?a=commit;h=304aaa2188a75e57afe36bad0443d90a9c715b8e

 language:
     name: German, reformed spelling
     tag:  de-1996

 hyphenmins:
     generation:
         left:  2
         right: 2
     typesetting:
         left:  2
         right: 2

 texlive:
     encoding: ec
     babelname: ngerman
     use_old_patterns_comment: Kept for the sake of backward compatibility, but newer and better patterns by WL are available.
     legacy_patterns: dehyphn.tex
     message: German hyphenation patterns (reformed orthography)
     package: german

 ===========================================================================
```

## en-us

```
 title: Hyphenation patterns for American English
 copyright: Copyright (C) 1990, 2004, 2005 Gerard D.C. Kuiken
 notice: This file is part of the hyph-utf8 package.
     See http://www.hyphenation.org/tex for more information.
 language:
     name: English, American spelling
     tag: en-us
 version: 2005-05-30
 authors:
   -
     name: Gerard D.C. Kuiken
 licence:
     text: >
         Copying and distribution of this file, with or without modification,
         are permitted in any medium without royalty provided the copyright
         notice and this notice are preserved.
 hyphenmins:
     typesetting:
         left: 2
         right: 3
 changes:
     March 1, 1990 Initial release
     May 30, 2005 Added copyright notice, no patterns change.
 texlive:
     encoding: ascii
     babelname: usenglishmax
     legacy_patterns: ushyphmax.tex
     message: Hyphenation patterns for American English
     package: english
 known_bugs:
     de-mo-c-rat: 'instead of dem-o-crat (see GitHub issue #15)'
 ==========================================

 ushyphmax.tex -- patterns for more hyphenation pattern memory (12000+).
 Also known as ushyphen.max.

 Needs extended pattern memory.
 Hyphenation trie becomes 7283 with 377 ops.

 These patterns are based on the Hyphenation Exception Log
 published in TUGboat, Volume 10 (1989), No. 3, pp. 337-341,
 and a large number of incorrectly hyphenated words not yet published.
 If added to Liang's before the closing bracket } of \patterns,
 the patterns run errorfree as far as known at this moment.

 These patterns find all admissible hyphens of the words in
 the Exception Log.  ushyph2.tex is a smaller set.

 Please send bugs or suggestions to tex-live (at) tug.org.

 2005-05-30 (karl): in the past, ushyphmax.tex was a file containing
 only the additional patterns, without the \patterns command, etc.
 This turned out not to be very useful, since in practice the TeX
 distributions need one self-contained file for a language.  Therefore,
 ushyphmax.tex now contains both the additional patterns from
 Dr. Kuiken, and the original patterns and hyphenations from Knuth's
 hyphen.tex.

 The Plain TeX hyphenation tables.
```
