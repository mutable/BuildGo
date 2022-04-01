{ pkgs, lib, ... }:
let
  inherit (pkgs) go;
  inherit (lib) nameValuePair;
  inherit (builtins)
    baseNameOf
    currentSystem
    derivation
    genericClosure
    listToAttrs
    placeholder
    replaceStrings
    removeAttrs
    unsafeDiscardStringContext
    ;

  builder = (derivation {
    name = "go-builder";
    system = currentSystem;
    builder = "${go}/bin/go";
    GOCACHE = "/tmp";
    args = [ "build" "-o" (placeholder "out") "${./builder.go}" ];
  }).outPath;

  # TODO(puck): check for duplicate values for packages!
  transitiveDeps = inDeps: map (a: a.value) (genericClosure {
    startSet = map (a: { key = a.gopkg.goImportPath; value = a; }) inDeps;
    operator = a: map (a: { key = a.gopkg.goImportPath; value = a; }) a.value.gopkg.goDeps;
  });

  # builds a map of original name => path in store, for source mapping reasons.
  # needs to use unsafeDiscardStringContext for some paths already in the store.
  namePaths = paths: listToAttrs (map (a: nameValuePair (unsafeDiscardStringContext (baseNameOf a)) "${a}") paths);

  callBuilder = { mode, ... }@params:
    let
      sanitizedName = replaceStrings [ "/" ] [ "_" ] (params.path or params.name);
    in
    derivation {
      name = if mode == "link" then sanitizedName else "go-${mode}-${sanitizedName}";
      inherit builder;
      system = currentSystem;
      __structuredAttrs = true;

      params = params // { goRoot = "${go}"; };
    };

  # calls the builder to build a package library.
  #
  # isProgram sets the path used in the callstack to the to path, but compiles the code
  # at the root, as is expected by the Go linker.
  makePackage = { isProgram, allDeps }: {
                                          # the import path of this package
                                          path
                                        , # the input .go files
                                          srcs
                                        , # the input .s files
                                          s_srcs ? [ ]
                                        }: callBuilder {
    mode = "compile";
    inherit path isProgram;
    includePath = map (a: a.gopkg) allDeps;
    inputFiles = namePaths srcs;
    inputAssemblyFiles = namePaths s_srcs;
  };


  # builds a package; output is at $out/${name}.a
  package =
    {
      # the import path of this package
      path
    , # the dependencies of this package, each containing a .gopkg, .goDeps, and .goImportPath
      deps ? [ ]
    , ...
    }@args:
    let
      allDeps = transitiveDeps deps;
    in
    let
      gopkg = (makePackage { isProgram = false; inherit allDeps; } (removeAttrs args [ "deps" ]))
        // {
        goImportPath = path;
        goDeps = allDeps;
        gopkg = gopkg;
      };
    in
    gopkg;

  # builds a Go program, similar to buildGo's program attrset.
  #
  # works by first building the package at an empty package path, and then
  # linking it immediately after.
  program = { name ? baseNameOf path, path, deps ? [ ], x_defs ? { }, ... }@args:
    let
      allDeps = transitiveDeps deps;

      archive = makePackage { isProgram = true; inherit allDeps; } (removeAttrs args [ "deps" "name" "x_defs" ]);
      program = callBuilder {
        mode = "link";
        inherit name;
        archive = "${archive}/${path}.a";
        includePath = allDeps;
        xDefs = x_defs;
      };

      gobin = program // { gobin = gobin; };
    in
    gobin;

in
{ inherit program package; }
