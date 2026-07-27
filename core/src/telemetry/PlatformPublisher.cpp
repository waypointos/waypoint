#include "telemetry/PlatformPublisher.hpp"

#include "util/Stamp.hpp"

namespace wp::telemetry {

waypoint::v1::PlatformInfo buildPlatformInfo(const wp::platform::Descriptor& d) {
    waypoint::v1::PlatformInfo info;
    info.set_platform_id(d.platformId);
    info.set_name(d.platformName);
    info.set_vehicle_class(d.vehicleClass);
    info.set_schema(static_cast<std::uint32_t>(d.schema));
    for (const auto& j : d.joints) {
        auto* pj = info.add_joints();
        pj->set_name(j.name);
        pj->set_bus_id(j.busId);
        pj->set_type(j.type);
        pj->set_ownership(j.ownership);
        pj->set_invert(j.invert);
        for (const auto& ci : j.commandInterfaces) pj->add_command_interfaces(ci);
    }
    if (d.kinematics) {
        auto* k = info.mutable_kinematics();
        k->set_model(d.kinematics->model);
        k->set_wheel_radius_m(d.kinematics->wheelRadiusM);
        k->set_track_width_m(d.kinematics->trackWidthM);
        for (const auto& [pos, name] : d.kinematics->wheels)
            (*k->mutable_wheels())[pos] = name;
    }
    return info;
}

void PlatformPublisher::publish() {
    wp::util::fillStamp(msg_.mutable_stamp());
    std::string out;
    msg_.SerializeToString(&out);
    nc_->publish(subject_, out.data(), out.size());
}

}  // namespace wp::telemetry
