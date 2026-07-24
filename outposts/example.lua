-- outposts/example.lua — example remote outpost definition
-- Copy this file to create your own outpost definitions.
-- The filename (minus .lua) becomes the outpost slug.
function meta()
    return {
        name        = "Example Outpost",
        url         = "https://example.com/cgi-bin/scoutpost",
        token       = "changeme",
        skip        = true,
        notify_down = true,
    }
end
