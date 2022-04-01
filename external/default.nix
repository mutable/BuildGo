{ platform, pkgs, ... }:
let
  inherit (platform.lib.buildGo) package program;
  inherit (pkgs) go lib;

  inherit (builtins)
    currentSystem
    derivation
    elemAt
    filter
    foldl'
    fromJSON
    genList
    head
    length
    readFile
    replaceStrings
    split
    ;
  inherit (lib)
    attrByPath
    recursiveUpdate
    splitString
    ;


  # outputs a derivation for the info of the packages rooted in `dir`, at package path
  # `path`.
  loadPackageInfo =
    let
      dumper = program {
        path = "github.com/mutable/buildGo/external";
        name = "dumper";

        srcs = [
          ./dumper.go
        ];
      };
    in
    { path, dir }: derivation {
      name = "pkginfo-" + (replaceStrings [ "/" ] [ "_" ] path);
      system = currentSystem;
      builder = "${dumper.gobin}/bin/dumper";
      args = [ dir path ];
    };

  # standard library packages do not have a dot in the first element, so we can
  # use that as a check for a package being built in.
  isBuiltin = path: length (split "\\." (head (makePathArray path))) == 1;

  # turns a path "example.com/foo/bar" into ["example.com" "foo" "bar"]
  makePathArray = splitString "/";

  # takes a list of { "example.com".foo.bar = [...] } style attrsets and folds
  # them together.
  foldPackages = foldl' recursiveUpdate { };

  # makeNested <nameList> <gopkg> builds the attrset shape expected by
  # foldPackages.
  makeNested = name: val:
    let
      nameList = makePathArray name;
      len = length nameList;
      reversedParts = genList (a: elemAt nameList (len - 1 - a)) len;
    in
    foldl' (val: name: { ${name} = val; }) val reversedParts;

  # The external interface. Takes a path, src, deps.
  #
  # It loads in the deps (finding the import path via .gopkg.goImportPath),
  # builds a list of attrsets expected by foldPackages, then folds both the
  # internal and external deps into one attrset, using that to resolve deps for
  # the packages it outputs.
  external = { path, src, deps ? [ ] }:
    let
      # makes a .package representatino
      packageForData = data:
        let
          getDep = dep: attrByPath (makePathArray dep) (throw "unknown dependency ${dep} (in ${data.path}") allDeps;
          externalIncludes = filter (a: !(isBuiltin a)) (data.sources.includes or [ ]);
        in
        (if data.isProgram then program else package) {
          inherit (data) path;
          srcs = map (a: src + "/${data.dir}/${a}") (data.sources.goFiles or [ ]);
          s_srcs = map (a: src + "/${data.dir}/${a}") (data.sources.sFiles or [ ]);
          deps = map getDep externalIncludes;
        };

      packageInfo = fromJSON (readFile (loadPackageInfo { path = path; dir = src; }));

      externalDeps = map (a: makeNested a.gopkg.goImportPath a) deps;
      internalDeps = map (data: makeNested data.path (packageForData data)) packageInfo;

      allDeps = foldPackages (externalDeps ++ internalDeps);
    in
    attrByPath (makePathArray path) (throw "internal error") (foldPackages internalDeps);
in
external
