# Test cases from Apache FOP 2.11

Source: `fop-2.11-src.tar.gz` from archive.apache.org (SHA-512
`9eacdee6d81ecf5af02c6266d20f55642febfa10bc9f3754cfb4d1532743899759d6bc7d362e80f3a1d8d370c54485e455d0ca3eac8b6604a05d3326ffefa00c`),
directory `fop/test/layoutengine/standard-testcases`. License: Apache License
2.0, see `LICENSE` and `NOTICE` in this directory. Every file under
`cases` keeps the license header of the test case.

`cases` contains the FO part of the test cases within the subset supported
by pdx. Variables (`##name`) are substituted as in FOP's `testcase2fo.xsl`.
The FOP checks (`<checks>`) refer to FOP's area tree and are not carried
over. Instead, `expect` holds for each case what FOP produces:
page count, line texts, baselines and line starts in points.

Cases outside the subset, cases that FOP itself disables, and cases that
need images from FOP's test directory are listed with a reason in
`../disabled.txt`.

Reimport and generate the expectations:

```bash
PDX_SUITE_IMPORT_FOP=<path>/fop-2.11/fop/test/layoutengine go test -run TestImportFOP ./internal/fosuite
```

```bash
make fo-reference
```
