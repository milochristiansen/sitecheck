-- outposts/local.lua — overrides for the implicit local outpost.
-- Level-only declaration: the local outpost renders at the basic level in the alternate site.
function meta()
    return {
        sites = {
            alternate = "basic",
        },
    }
end
