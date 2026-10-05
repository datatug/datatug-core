# filestore

An implementation of a [store](..) that persist and retrieves DataTug projects. 

The file store writes only plain files in plain folders of the project: it
does not write through a link. Every write, and every delete, of a project file
or folder walks from the project folder down to the target (internal/plainfs)
and refuses a link, or an entry that is not a plain file or folder, anywhere on
the way; the refusal names the path inside the project. Reads are not covered.
