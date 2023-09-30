# HTMLsync
HTMLsync is a tool that managed all the HTML section, header and footer elements in a HTML directory, keeping them in sync as you work.

When HTMLsync is first executed in a HTML directory, it adds a hash attribute to these elements, and adds *id* attributes if not present.

After editing HTML, when HTMLsync is re-run, it identifies which elements have changed due to a hash mismatch, and updates elements with the same id. Where multiple sections have changed, it prompts the user which section should be used.

## How to use it
1. First build it:
    ```
    $ git clone https://github.com/dblueman/htmlsync
    $ cd htmlsync
    $ go install
    ```
1. [optional] Run HTMLsync from a directory of HTML files to initially reflow the HTML:
    ```
    $ cd webroot
    $ htmlsync --reformat
    ```
1. After editing one or more HTML section, header or footer elements, propagate change to all files:
    ```
    $ htmlsync
    ```
1. HTMLsync will ask you to select which HTML section where there are multiple unique ones

## How it works
1. HTMLsync traverses all HTML files recursively, identifying HTML section, header and footer elements (*sections*)
1. It builds a global list of sections, storing hash, id and node pointer
1. A *data-htmlsync* hash attrbute is added to each section (id and hash are skipped during hash calculation)

If --reformat was used:
 1. sections with same hash are updated with the same id
 1. sections with different hash but same id are given unique id

otherwise:
 1. sections with same id are iterated
 1. if more than one has been updated (new hash != old hash), ask used which section to use
 1. update all sections with the chosen section
 1. a list of sections ids is shown across all files
 1. all HTML files are rewritten