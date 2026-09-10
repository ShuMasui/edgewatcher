package com.edgewatcher.di;

import android.content.Context;
import com.edgewatcher.domain.port.AlarmScheduler;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata("javax.inject.Singleton")
@QualifierMetadata("dagger.hilt.android.qualifiers.ApplicationContext")
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_AlarmsFactory implements Factory<AlarmScheduler> {
  private final Provider<Context> contextProvider;

  private AppModule_AlarmsFactory(Provider<Context> contextProvider) {
    this.contextProvider = contextProvider;
  }

  @Override
  public AlarmScheduler get() {
    return alarms(contextProvider.get());
  }

  public static AppModule_AlarmsFactory create(Provider<Context> contextProvider) {
    return new AppModule_AlarmsFactory(contextProvider);
  }

  public static AlarmScheduler alarms(Context context) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.alarms(context));
  }
}
